// Copyright © 2018 JZM.io
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package buildinfo

import (
	"fmt"
	"runtime"
	"strings"
)

// CommitHash set by linker with `-X` flag
var CommitHash string

// BuildDate set by linker with `-X` flag
var BuildDate string

// Version semantic
type Version struct {
	// Major version increment for backwards-incompatible changes.
	Major int
	// Minor version increment for new features.
	Minor int
	// Patch version increment for bug fixes.
	Patch int
}

var currentVersion = Version{
	Major: 0,
	Minor: 2,
	Patch: 2,
}

// String interface
func (v Version) String() string {
	return fmt.Sprintf("%v.%v.%v", v.Major, v.Minor, v.Patch)
}

// VersionString returns a string represents version and os info
func VersionString() string {
	var sb strings.Builder
	sb.WriteString("v")
	sb.WriteString(currentVersion.String())
	if CommitHash != "" {
		sb.WriteString("-")
		sb.WriteString(CommitHash)
	}
	sb.WriteString(" ")

	if BuildDate != "" {
		sb.WriteString("(")
		sb.WriteString(BuildDate)
		sb.WriteString(")")
		sb.WriteString(" ")
	}

	sb.WriteString(runtime.GOOS)
	sb.WriteString("/")
	sb.WriteString(runtime.GOARCH)

	return sb.String()
}
