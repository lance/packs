PACK_CMD?=pack

GIT_TAG := $(shell git tag --points-at HEAD)
VERSION_TAG := $(shell [ -z $(GIT_TAG) ] && echo 'tip' || echo $(GIT_TAG) )

.PHONY: buildpacks publish test

all: buildpacks

buildpacks:
	./hack/make.sh buildpacks $(VERSION_TAG)

publish:
	./hack/make.sh publish $(VERSION_TAG)

test: bin/func_snapshot buildpacks
	go run test/test_buildpacks.go $(VERSION_TAG)

bin/func_snapshot:
	hack/install-func-snapshot.sh
