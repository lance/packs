///usr/bin/true; exec /usr/bin/env go run "$0" "$@"

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

const repo = "ghcr.io/boson-project"

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs
		cancel()
		<-sigs
		os.Exit(1)
	}()

	version := os.Args[1]
	err := runTests(ctx, version)
	if err != nil {
		fmt.Printf("::error::%s\n", err.Error())
		os.Exit(1)
	}
}

var buildpacks = []struct {
	Buildpacks []string
	Runtimes   []string
}{
	{
		Buildpacks: []string{"ghcr.io/boson-project/go-function-buildpack", "paketo-buildpacks/go-dist"},
		Runtimes:   []string{"go"},
	},
	{
		Buildpacks: []string{"ghcr.io/boson-project/typescript-function-buildpack", "paketo-buildpacks/nodejs"},
		Runtimes:   []string{"typescript"},
	},
}

func runTests(ctx context.Context, version string) error {

	os.Setenv("FUNC_REGISTRY", repo)

	for i := range buildpacks {
		buildpacks[i].Buildpacks[0] = fmt.Sprintf("%s:%s", buildpacks[i].Buildpacks[0], version)
	}

	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	packCmd := "pack"
	if pc, ok := os.LookupEnv("PACK_CMD"); ok {
		packCmd = pc
	}

	funcBinaries := []string{
		filepath.Join(wd, "bin", "func_snapshot"),
	}
	templates := []string{
		"http",
		"events",
	}

	oldWD, err := os.Getwd()
	if err != nil {
		return err
	}

	testDir, err := os.MkdirTemp("", "test_func")
	if err != nil {
		return err
	}
	// defer os.RemoveAll(testDir)

	err = os.Chdir(testDir)
	if err != nil {
		return err
	}
	defer os.Chdir(oldWD)

	// just a counter to avoid name collision
	var fnNo int

	for _, funcBinary := range funcBinaries {
		for _, buildpack := range buildpacks {
			for _, runtime := range buildpack.Runtimes {
				for _, template := range templates {
					fnName := fmt.Sprintf("fn-%s-%s-%d", runtime, template, fnNo)
					if !runTest(ctx, packCmd, funcBinary, runtime, template, fnName, buildpack.Buildpacks) {
						return fmt.Errorf("some test failed")
					}

					fnNo++
				}
			}
		}
	}
	return nil
}

func runTest(ctx context.Context, packCmd, funcBinary, runtime, template, fnName string, buildpacks []string) (succeeded bool) {
	start := time.Now()

	fmt.Printf("[RUNNING TEST]\nbuildpacks: %s\nruntime: %s\ntemplate: %s\nfunc: %s\n",
		buildpacks, runtime, template, funcBinary)

	var errs []error

	defer func() {
		fmt.Printf("duration: %s\n", time.Since(start))
		if len(errs) > 0 {
			fmt.Println("❌ Failure")
			for _, e := range errs {
				fmt.Printf("::error::%s\n", e.Error())
			}
			succeeded = false
		} else {
			fmt.Println("✅ Success")
			succeeded = true
		}
	}()

	fmt.Println("::group::Output")
	defer fmt.Println("::endgroup::")

	runCmd := func(name string, arg ...string) error {

		cmdCtx, cmdCancel := context.WithCancel(context.Background())
		cmd := exec.CommandContext(cmdCtx, name, arg...)
		cmd.Stdout = os.Stdout
		errOut := bytes.NewBuffer(nil)
		cmd.Stderr = io.MultiWriter(os.Stdout, errOut)
		cmd.Start()
		go func() {
			<-ctx.Done()
			cmd.Process.Signal(os.Interrupt)
			time.Sleep(time.Second * 10)
			cmdCancel()
		}()
		err := cmd.Wait()
		if err != nil {
			return fmt.Errorf("%w (stderr: %q)", err, errOut.String())
		}
		return nil
	}

	err := runCmd(
		funcBinary,
		"create", fnName,
		"--runtime", runtime,
		"--template", template,
		"--verbose")
	if err != nil {
		e := fmt.Errorf("failed to create a function: %w", err)
		fmt.Println(e)
		errs = append(errs, e)
		return
	}

	err = runCmd(
		packCmd,
		"--trust-builder=false",
		"build", repo+"/"+fnName+":latest",
		"--builder", "paketobuildpacks/builder:base",
		"--buildpack", buildpacks[1],
		"--buildpack", buildpacks[0],
		"--verbose",
		"--path", fnName)
	if err != nil {
		e := fmt.Errorf("failed to build the function using `pack` (--trust-builder=false): %w", err)
		fmt.Println(e)
		errs = append(errs, e)
	}

	err = runCmd(
		packCmd,
		"--trust-builder=true",
		"build", repo+"/"+fnName+":latest",
		"--builder", "paketobuildpacks/builder:base",
		"--buildpack", buildpacks[1],
		"--buildpack", buildpacks[0],
		"--verbose",
		"--path", fnName)
	if err != nil {
		e := fmt.Errorf("failed to build the function using `pack` (--trust-builder=true): %w", err)
		fmt.Println(e)
		errs = append(errs, e)
	}

	return
}
