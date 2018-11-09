package main

import (
	"os"

	"github.com/master-g/protoc-gen-bean/pkg/shared"

	"github.com/golang/protobuf/proto"
	"github.com/master-g/protoc-gen-bean/pkg/generator"
)

func main() {
	// Begin by allocating a generator. The request and response structures are stored there
	// so we can do error handling easily - the response structure contains the field to
	// report failure.
	g := generator.New()

	var data []byte
	var err error
	// data, err = ioutil.ReadAll(os.Stdin)
	// if err != nil {
	// 	g.Error(err, "reading input")
	// }
	// shared.Dump(data, "dump.txt")
	data = shared.ProtocDump

	if err := proto.Unmarshal(data, g.Request); err != nil {
		g.Error(err, "parsing input proto")
	}

	if len(g.Request.FileToGenerate) == 0 {
		g.Fail("no files to generate")
	}

	g.CommandLineParameters(g.Request.GetParameter())

	// Create a wrapped version of the Descriptors and EnumDescriptors that
	// point to the file that defines them.
	g.WrapTypes()
	g.BuildTypeNameMap()

	g.GenerateAllFiles()

	// Send back the results.
	data, err = proto.Marshal(g.Response)
	if err != nil {
		g.Error(err, "failed to marshal output proto")
	}
	_, err = os.Stdout.Write(data)
	if err != nil {
		g.Error(err, "failed to write output proto")
	}
}
