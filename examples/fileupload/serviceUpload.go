package main

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"github.com/graphql-go/graphql"
	handler "github.com/jpascal/graphql-upload"
	uuid "github.com/satori/go.uuid"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
)

var UploadType = graphql.NewScalar(graphql.ScalarConfig{
	Name:        "Upload",
	Description: "Scalar upload object",
})

type FileWrapper struct {
	File *os.File
	Name string
}

var File = graphql.NewObject(graphql.ObjectConfig{
	Name:        "File",
	Description: "File object",
	Fields: graphql.Fields{
		"name": &graphql.Field{
			Type: graphql.String,
			Resolve: func(params graphql.ResolveParams) (interface{}, error) {
				file := params.Source.(*FileWrapper)
				name := path.Base(file.Name)

				return name, nil
			},
		},
		"hash": &graphql.Field{
			Type: graphql.String,
			Resolve: func(params graphql.ResolveParams) (interface{}, error) {
				file := params.Source.(*FileWrapper)
				if data, err := ioutil.ReadAll(file.File); err == nil {
					fileHash := sha1.Sum(data)

					return hex.EncodeToString(fileHash[:]), nil
				} else {
					return nil, err
				}

			},
		},
		"size": &graphql.Field{
			Type: graphql.Int,
			Resolve: func(params graphql.ResolveParams) (interface{}, error) {
				file := params.Source.(*FileWrapper)
				if info, err := file.File.Stat(); err != nil {
					return nil, err
				} else {
					return info.Size(), nil
				}
			},
		},
	},
})

func main() {
	schema, err := graphql.NewSchema(
		graphql.SchemaConfig{
			Query: graphql.NewObject(
				graphql.ObjectConfig{
					Name: "Query",
					Fields: graphql.Fields{
						"file": &graphql.Field{
							Type: File,
							Args: graphql.FieldConfigArgument{
								"id": &graphql.ArgumentConfig{
									Type: graphql.NewNonNull(graphql.String),
								},
							},
							Resolve: func(params graphql.ResolveParams) (interface{}, error) {
								if fileId, ok := params.Args["id"].(string); ok {
									fileUuid, err := uuid.FromString(fileId)
									if err != nil {
										return nil, err
									}

									file, err := os.Open("tmp/" + fileUuid.String())
									if err != nil {
										return nil, err
									}

									return &FileWrapper{File: file, Name: fileUuid.String()}, nil
								} else {
									return nil, errors.New("file id is not provided")
								}
							},
						},
					},
				}),
			Mutation: graphql.NewObject(
				graphql.ObjectConfig{
					Name: "Mutation",
					Fields: graphql.Fields{
						"upload": &graphql.Field{
							Type: graphql.String,
							Args: graphql.FieldConfigArgument{
								"file": &graphql.ArgumentConfig{
									Type: UploadType,
								},
							},
							Resolve: func(params graphql.ResolveParams) (interface{}, error) {
								upload, uploadPresent := params.Args["file"].(handler.File)
								if uploadPresent {
									id := uuid.NewV4().String()
									targetFile, err := os.Create("tmp/" + id)
									if err != nil {
										return nil, err
									}

									defer targetFile.Close()
									nBytes, err := io.Copy(targetFile, upload.File)
									if err != nil {
										return nil, err
									}

									log.Println("File saved nBytes: ", nBytes)
									return id, nil
								} else {
									return nil, errors.New("no file found in request")
								}

							},
						},
					},
				}),
		})
	if err != nil {
		panic(err)
	}

	server := &http.Server{Addr: "0.0.0.0:5000", Handler: handler.New(func(request *handler.Request) interface{} {
		return graphql.Do(graphql.Params{
			RequestString:  request.Query,
			OperationName:  request.OperationName,
			VariableValues: request.Variables,
			Schema:         schema,
			Context:        request.Context,
		})
	}, &handler.Config{
		MaxBodySize: 1024,
	}),
	}
	server.ListenAndServe()
}