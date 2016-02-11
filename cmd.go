package main

import (
	"flag"
	"fmt"
	"go/build"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

var outputTemplate = template.Must(template.New("script").Parse(
	`{{$cwd := .Cwd}}# Automatically abort on error
set -e

# Create temporary directory to simulate empty GOPATH. 'go get' will download
# the dependencies into this path where we later copy them from.
tmpDir="$(mktemp --directory)"

# Remove currently installed vendored dependencies
rm -rf {{.Cwd}}/vendor/*

# Setup environment for 'go get'
export GO15VENDOREXPERIMENT=0
export GOPATH=$tmpDir

# Setup link from temporary GOPATH to the current package
mkdir -p $GOPATH/src/{{.Package}}
rm -r $GOPATH/src/{{.Package}}
ln -s {{$cwd}} $GOPATH/src/{{.Package}}

cd $GOPATH/src/{{.Package}}

# Download depenencies into temporary directory
installedPackagesStr=""
{{range .Packages}}installedPackagesStr+="$(go get -d -v -t {{.}} 2>&1)"
installedPackagesStr+=$'\n'
{{end}}
# Remove symlink
rm -rf $GOPATH/src/{{.Package}}

# Move vendored depenencies from temporary storage into current project
rsync -r $tmpDir/src/ {{.Cwd}}/vendor

{{if .AddSubmodules}}# Set currently used working directory
cwd="{{.Cwd}}/"

readarray installedPackages <<< "$installedPackagesStr"
for entry in "${installedPackages[@]}"
do
  # The array may contain empty elements which we want to filter out
  entry="$(echo "$entry" | tr -d '\n')"
  if [[ -z "$entry" ]]; then
    continue
  fi

  # 'go get' output lines such as 'github.com/tus/usd (download)' but we are
  # only interested in the actual import path
  pkg="$(echo "$entry" | cut -d' ' -f 1)"

  # Attempt to find a remote URL which we can use to add this package as a
  # submodule
  remoteUrl="$(git -C {{$cwd}}/vendor/$pkg config --get remote.origin.url || true)"
  if [ -n "$remoteUrl" ]; then
    # Resolve the absolute path to a relative one for .gitmodules
    # We do not want paths such as /home/marius/go/src/foo/vendor/bla but
    # just vendor/bla. In addition, we do not want subpackages added as
    # submodules, e.g. only github.com/aws/aws-go-sdk and not
    # github.com/aws/aws-go-sdk/service/s3
    toplevelDir="$(git -C {{$cwd}}/vendor/$pkg rev-parse --show-toplevel)"
    resolvedDir="${toplevelDir#"$cwd"}"
    git -C "{{$cwd}}" submodule add -f $remoteUrl "$resolvedDir" || true

    # If the submodule has not been added to .gitmodules, yet, we will do it
    # manually.
    grep -q "path = $resolvedDir" "{{$cwd}}/.gitmodules" || {
      echo "[submodule \"vendor/$pkg\"]" >> "{{$cwd}}/.gitmodules";
      echo "	path = $resolvedDir" >> "{{$cwd}}/.gitmodules";
      echo "	url = $remoteUrl" >> "{{$cwd}}/.gitmodules";
    }
  fi
done
{{else}}# Adding submodules disabled
{{end}}
# Remove temporary package installation directory
rm -rf $tmpDir
`))

type templateInput struct {
	Cwd           string
	Packages      []string
	AddSubmodules bool
	Package       string
}

func main() {
	var destPath string
	var recursive bool
	var addSubmodules bool
	flag.StringVar(&destPath, "directory", "./", "Directory to analyse dependencies and download to")
	flag.BoolVar(&recursive, "recursive", true, "Indicates whether current project should be analysed recursively")
	flag.BoolVar(&addSubmodules, "submodules", true, "Indicates whether dependencies should be added as Git submodules")

	flag.Parse()

	destPathAbs, err := filepath.Abs(destPath)
	handleErr(err)

	pkgImportPath := ""

	if GOPATH := os.Getenv("GOPATH"); GOPATH != "" {
		if !os.IsPathSeparator(GOPATH[len(GOPATH)-1]) {
			GOPATH += string(os.PathSeparator)
		}

		GOPATH += "src/"

		if strings.HasPrefix(destPathAbs, GOPATH) {
			pkgImportPath = destPathAbs[len(GOPATH):]
		}
	}

	externalImports := make([]string, 0)

	walkFunc := func(path string, info os.FileInfo, err error) error {
		handleErr(err)

		base := filepath.Base(path)
		// Exclude special directories from analysing
		if base == ".git" || base == "vendor" {
			return filepath.SkipDir
		}

		if !info.IsDir() {
			return nil
		}

		_, err = build.ImportDir(path, 0)
		if _, ok := err.(*build.NoGoError); !ok {
			handleErr(err)
		} else {
			// Ignore directory if no Go source files are in there
			return nil
		}

		externalImports = append(externalImports, "./"+path)

		return nil
	}

	if recursive {
		handleErr(filepath.Walk(destPath, walkFunc))
	} else {
		info, err := os.Stat(destPath)
		handleErr(err)
		handleErr(walkFunc(destPath, info, nil))
	}

	if len(externalImports) == 0 {
		return
	}

	cwd, err := os.Getwd()
	handleErr(err)

	outputTemplate.Execute(os.Stdout, templateInput{
		Cwd:           cwd,
		Packages:      externalImports,
		AddSubmodules: addSubmodules,
		Package:       pkgImportPath,
	})
}

func handleErr(err error) {
	if err == nil {
		return
	}

	fmt.Fprintf(os.Stderr, "Internal error occured:\n\t%s\n", err)

	os.Exit(1)
}
