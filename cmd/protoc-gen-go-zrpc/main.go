package main

import (
	"flag"
	"fmt"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

const (
	version = "v0.1.0"
)

var (
	requireUnimplemented *bool
)

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Printf("protoc-gen-go-zrpc %v\n", version)
		return
	}

	var flags flag.FlagSet

	requireUnimplemented = flag.Bool("require_unimplemented_servers", true, "set to false to match legacy behavior")

	protogen.Options{
		ParamFunc: flags.Set,
	}.Run(func(plugin *protogen.Plugin) error {
		plugin.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL) | uint64(pluginpb.CodeGeneratorResponse_FEATURE_SUPPORTS_EDITIONS)
		plugin.SupportedEditionsMinimum = descriptorpb.Edition_EDITION_PROTO2
		plugin.SupportedEditionsMaximum = descriptorpb.Edition_EDITION_2024
		for _, f := range plugin.Files {
			if !f.Generate {
				continue
			}
			generateFiles(plugin, f)
		}
		return nil
	})
}
