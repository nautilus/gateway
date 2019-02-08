task "install:ci" {
    description = "install the necessary dependencies to run in CI. does not run `install`"
    command     = <<EOF
    go get \
        golang.org/x/tools/cmd/cover \
        github.com/mattn/goveralls \
        github.com/tcnksm/ghr \
        github.com/mitchellh/gox
    EOF
}

task "install" {
    description = "install the dependencies to develop locally"
    command     = "go get -v {% .files %}"
}

task "test" {
    description = "run the tests"
    command     = "go test {% .files %}"
}

task "test:coverage" {
    description = "run the tests, generate a coverage report, and report it to coveralls"
    pipeline    = [
        "go test -v -covermode=atomic -coverprofile=coverage.out {% .files %}",
        "$HOME/gopath/bin/goveralls -coverprofile=coverage.out -service=travis-ci -repotoken $COVERALLS_TOKEN"
    ]
}

task "build" {
    description = "build executable in all supported architectures"
    command     = "gox -os=\"linux darwin windows\" -arch=\"amd64\" -output=\"bin/gateway_{{.OS}}_{{.Arch}}\" -verbose ./cmd/..."
}

task "deploy" {
    description = "push the built artifacts to the release. assumes its running in CI"
    command = "ghr -t $GITHUB_TOKEN  -u \"nautilus\" -r \"gateway\" -delete $TRAVIS_TAG ./bin"
}

variables {
    files = "$(go list -v ./... | grep -iEv \"cmd|examples\")"
}

config {
    delimiters = ["{%", "%}"]
}
