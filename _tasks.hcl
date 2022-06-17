task "install:cd" {
    description = "Install the necessary dependencies to run in CI. does not run `install`"
    command     = <<EOF
    go install github.com/mitchellh/gox@latest
    EOF
}

task "install" {
    description = "Install the dependencies to develop locally"
    command     = "go mod download all"
}

task "tests" {
    description = "Run the tests"
    command     = "go test -race ./..."
}

task "tests:coverage" {
    description = "Run the tests, generate a coverage report, and report it to coveralls"
    pipeline    = [
        "go test -v -coverprofile=coverage.out -race ./...",
        "cd ./cmd/gateway && go test -v -race ./...",
    ]
}

task "build" {
    description = "Build executable in all supported architectures"
    command     =  <<EOF
        cd cmd/gateway && gox -os="linux darwin windows" -arch=amd64 -output="../../bin/gateway_{{.OS}}_{{.Arch}}" -verbose .
    EOF
}

config {
    // have to change the template delimiters to support gox
    delimiters = ["{%", "%}"]
}
