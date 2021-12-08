package shared

import (
	"fmt"
	"log"
	"os"
)

// Dump bytes to file
func Dump(data []byte, name string) {
	var err error
	var f *os.File
	f, err = os.Create(name)
	if err != nil {
		log.Fatal(err)
	}
	for i, v := range data {
		if i%16 == 0 {
			_, err = f.WriteString("\n")
			if err != nil {
				log.Fatal(err)
			}
		}
		_, err = f.WriteString(fmt.Sprintf("0x%02X,", v))
		if err != nil {
			log.Fatal(err)
		}
	}
	err = f.Sync()
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		err = f.Close()
		if err != nil {
			log.Fatal(err)
		}
	}()
}

// ProtocDump holds byte codes from protoc for debugging generator
var ProtocDump = []byte{}
