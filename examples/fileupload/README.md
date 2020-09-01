# File uploads

This example demonstrates proxying file upload requests according to specification:
 - https://github.com/jaydenseric/graphql-multipart-request-spec
 
## How to test

1. Run file upload service

```
cd examples/fileupload/

go run serviceUpload.go
```

2. Run the gateway
```
go run ./cmd/ start --port 4000 --services http://localhost:5000
```

3. Execute file upload query:

```
curl localhost:4000/graphql \
  -F operations='{ "query": "mutation ($someFile: Upload) { upload(file: $someFile) }", "variables": { "someFile": null } }' \
  -F map='{ "0": ["variables.someFile"] }' \
  -F 0=@README.md
```

4. Validate that file is uploaded to temporary folder:

```
ls examples/fileupload/tmp/
> cd4c0810-d5d7-4adf-9edb-bea74eadae4e

head -n3 examples/fileupload/tmp/*
> # nautilus/gateway
>  
>  ![CI Checks](https://github.com/nautilus/gateway/workflows/CI%20Checks/badge.svg?branch=master) [![Coverage Status](https://coveralls.io/repos/github/nautilus/gateway/badge.svg?branch=master)](https://coveralls.io/github/nautilus/gateway?branch=master) [![Go Report Card](https://goreportcard.com/badge/github.com/nautilus/gateway)](https://goreportcard.com/report/github.com/nautilus/gateway)
```

Todo:
- [ ] Test multiple file uploads
- [ ] If Upload field is not nullable gateway returns an error
- [ ] Write unit tests

