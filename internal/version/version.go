/*
MIT License

# Copyright (c) 2025 OcomSoft

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

// Package version provides version and build information for the makemigrations CLI.
package version

import (
	"fmt"
	"runtime"
)

// Build information set by ldflags during compilation
var (
	// Version represents the current morphic version
	// This variable is updated by bumpversion during releases or set via ldflags
	Version = "1.14.0"

	// BuildDate is set via ldflags during build
	BuildDate = "unknown"

	// GitCommit is set via ldflags during build
	GitCommit = "unknown"
)

// GetVersion returns the current version string
func GetVersion() string {
	return Version
}

// GetDisplayVersion returns the formatted version for display
func GetDisplayVersion() string {
	return fmt.Sprintf("morphic v%s", Version)
}

// GetFullVersion returns detailed version information
func GetFullVersion() string {
	return fmt.Sprintf("morphic v%s (built %s, commit %s, %s/%s)",
		Version, BuildDate, GitCommit, runtime.GOOS, runtime.GOARCH)
}

// GetBuildInfo returns build information
func GetBuildInfo() map[string]string {
	return map[string]string{
		"version":   Version,
		"buildDate": BuildDate,
		"gitCommit": GitCommit,
		"goVersion": runtime.Version(),
		"platform":  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		"compiler":  runtime.Compiler,
	}
}
