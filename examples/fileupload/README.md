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
  -F operations='{ "query": "mutation ($someFile: Upload!) { upload(file: $someFile) }", "variables": { "someFile": null } }' \
  -F map='{ "0": ["variables.someFile"] }' \
  -F 0=@README.md
```

4. Validate that the file is uploaded to temporary folder:

```
ls examples/fileupload/tmp/
> cd4c0810-d5d7-4adf-9edb-bea74eadae4e

head -n3 examples/fileupload/tmp/*
> # nautilus/gateway
>  
>  ![CI Checks](https://github.com/nautilus/gateway/workflows/CI%20Checks/badge.svg?branch=master) [![Coverage Status](https://coveralls.io/repos/github/nautilus/gateway/badge.svg?branch=master)](https://coveralls.io/github/nautilus/gateway?branch=master) [![Go Report Card](https://goreportcard.com/badge/github.com/nautilus/gateway)](https://goreportcard.com/report/github.com/nautilus/gateway)
```

5. Execute multi file upload query:
```
curl localhost:4000/graphql \
  -F operations='{"query":"mutation TestFileUpload(\n  $someFile: Upload!,\n\t$allFiles: [Upload!]!\n) {\n  upload(file: $someFile)\n  uploadMulti(files: $allFiles)\n}","variables":{"someFile":null,"allFiles":[null,null]},"operationName":"TestFileUpload"}' \
  -F map='{"0":["variables.someFile"],"1":["variables.allFiles.0"],"2":["variables.allFiles.1"]}' \
  -F 0=@README.md \
  -F 1=@go.mod \
  -F 2=@go.sum
```

6. Validate that more files are created in the folder:

```
ls -la examples/fileupload/tmp/

> -rw-rw-r-- 1 user user   924 Sep  3 00:12 343b9067-f2be-4ea9-b73b-4e8390ed55c7
> -rw-rw-r-- 1 user user  1557 Sep  3 00:12 5417f766-8e7d-44ef-afb6-90ec0b4c548c
> -rw-rw-r-- 1 user user 15089 Sep  3 00:12 a590196f-6450-4785-8998-8013ff7c8cf3
```

Todo:
- [ ] Write unit tests
- [ ] Test batch mode

