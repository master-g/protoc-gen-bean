package shared

import (
	"fmt"
	"os"
)

// Dump bytes to file
func Dump(data []byte, name string) {
	f, err := os.Create(name)
	if err != nil {
		os.Exit(0)
	}
	for i, v := range data {
		if i%16 == 0 {
			f.WriteString("\n")
		}
		f.WriteString(fmt.Sprintf("0x%02X,", v))
	}
	f.Sync()
	defer f.Close()
}

// ProtocDump holds byte codes from protoc for debugging generator
var ProtocDump = []byte{}
