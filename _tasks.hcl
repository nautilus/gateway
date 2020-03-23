task "install:ci" {
    description = "Install the necessary dependencies to run in CI. does not run `install`"
    command     = <<EOF
    go get \
        github.com/tcnksm/ghr \
        github.com/mitchellh/gox
    EOF
}

task "install" {
    description = "Install the dependencies to develop locally"
    command     = "go get -v {% .files %}"
}

task "tests" {
    description = "Run the tests"
    command     = "go test {% .files %}"
}

task "tests:coverage" {
    description = "Run the tests, generate a coverage report, and report it to coveralls"
    pipeline    = [
        "go test -v -covermode=atomic -coverprofile=coverage.out {% .files %}",
    ]
}

task "build" {
    description = "Build executable in all supported architectures"
    command     =  <<EOF
        gox -os="linux darwin windows" -arch=amd64 -output="bin/gateway_{{.OS}}_{{.Arch}}" -verbose ./cmd/...
    EOF
}

task "deploy" {
    description = "Push the built artifacts to the release. assumes its running in CI"
    command     = "ghr -t $GITHUB_TOKEN -u nautilus -r gateway $INPUT_VERSION ./bin"
}

variables {
    files = "$(go list -v ./... | grep -iEv \"cmd|examples\")"
}

config {
    // have to change the template delimiters to support gox
    delimiters = ["{%", "%}"]
}
