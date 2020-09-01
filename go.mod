module github.com/nautilus/gateway

require (
	github.com/99designs/gqlgen v0.11.3
	github.com/graph-gophers/graphql-go v0.0.0-20190108123631-d5b7dc6be53b
	github.com/graphql-go/graphql v0.7.9 // indirect
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/konsorten/go-windows-terminal-sequences v1.0.2 // indirect
	github.com/mitchellh/mapstructure v1.2.2
	github.com/nautilus/graphql v0.0.10
	github.com/opentracing/opentracing-go v1.1.0 // indirect
	github.com/sirupsen/logrus v1.4.2
	github.com/spf13/cobra v0.0.3
	github.com/spf13/pflag v1.0.3 // indirect
	github.com/stretchr/testify v1.4.0
	github.com/vektah/gqlparser/v2 v2.0.1
	golang.org/x/net v0.0.0-20200320220750-118fecf932d8
	golang.org/x/sys v0.0.0-20200321134203-328b4cd54aae // indirect
	gopkg.in/yaml.v2 v2.2.8 // indirect
)

go 1.13

replace github.com/nautilus/graphql => github.com/obukhov/graphql v0.0.11-0.20200901225829-8b82c957270a
