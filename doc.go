// MIT License
//
// Copyright (c) 2018 Master.G
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

/*
	A plugin for the Google protocol buffer compiler to generate Java bean code.
	Run it by building this program and putting it in your path with the name
		protoc-gen-bean
	That word 'bean' at the end becomes part of the option string set for the
	protocol compiler, so once the protocol compiler (protoc) is installed
	you can run
		protoc --bean_out=output_directory input_directory/file.proto
	to generate java beans for the protocol defined by file.proto.
	With that input, the output will be written to
		output_directory/messages.java

	The generated code is documented in the package comment for
	the library.

	See the README and documentation for protocol buffers to learn more:
		https://developers.google.com/protocol-buffers/

*/
package documentation
