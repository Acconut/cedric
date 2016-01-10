package main

import (
  "io/ioutil"
  "os"
  "fmt"
  "go/build"
  "strings"
  "path/filepath"
  "text/template"
  "flag"
)

var outputTemplate = template.Must(template.New("script").Parse(
`{{$gopath := .Tmp}}{{$cwd := .Cwd}}# Remove currently installed vendored dependencies
rm -rf {{.Cwd}}/vendor/*

export GO15VENDOREXPERIMENT=0

installedPackagesStr=""

# Download depenencies into temporary directory
{{range .Packages}}installedPackagesStr+="$(GOPATH={{$gopath}} go get -d -v {{.}} 2>&1)"
installedPackagesStr+=$'\n'
{{end}}
# Move vendored depenencies from temporary storage into current project
rsync -r {{.Tmp}}/src/ {{.Cwd}}/vendor

{{if .AddSubmodules}}# Set currently used working directory
cwd="{{.Cwd}}/"

readarray installedPackages <<< "$installedPackagesStr"
for entry in "${installedPackages[@]}"
do
  entry="$(echo "$entry" | tr -d '\n')"
  if [[ -z "$entry" ]]; then
    continue
  fi

  pkg="$(echo "$entry" | cut -d' ' -f 1)"

  remoteUrl="$(git -C {{$cwd}}/vendor/$pkg config --get remote.origin.url)"
  if [ -n "$remoteUrl" ]; then
    toplevelDir="$(git -C {{$cwd}}/vendor/$pkg rev-parse --show-toplevel)"
    resolvedDir="${toplevelDir#"$cwd"}"
    git -C {{$cwd}} submodule add -f $remoteUrl $resolvedDir
    echo "[submodule \"$pkg\"]" >> {{$cwd}}/.gitmodules
    echo "  path = $resolvedDir" >> {{$cwd}}/.gitmodules
    echo "  url = $remoteUrl" >> {{$cwd}}/.gitmodules
  fi
done

# Add depenencies as submodules if possible{{range .Packages}}
#remoteUrl="$(git -C {{$cwd}}/vendor/{{.}} config --get remote.origin.url)"
#if [ -n "$remoteUrl" ]; then
#  toplevelDir="$(git -C {{$cwd}}/vendor/{{.}} rev-parse --show-toplevel)"
#  resolvedDir="${toplevelDir#"$cwd"}"
#  git -C {{$cwd}} submodule add $remoteUrl $resolvedDir
#fi
{{end}}{{else}}# Adding submodules disabled
{{end}}
# Remove temporary package installation directory
#rm -rf {{.Tmp}}

echo $installedPackages
`))

type templateInput struct {
  Cwd string
  Tmp string
  Packages []string
  AddSubmodules bool
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

    pkg, err := build.ImportDir(path, 0)
    if _, ok := err.(*build.NoGoError); !ok {
      handleErr(err)
    } else {
      return nil
    }

    imports := make([]string, len(pkg.Imports) + len(pkg.TestImports) + len(pkg.XTestImports))
    imports = append(imports, pkg.Imports...)
    imports = append(imports, pkg.TestImports...)
    imports = append(imports, pkg.XTestImports...)

    for _, pkgName := range imports {
      if pkgName == "" {
        continue
      }

      if strings.HasPrefix(pkgName, pkgImportPath) {
        continue
      }

      pkg, err := build.Import(pkgName, ".", build.AllowBinary)
      handleErr(err)

      if !pkg.Goroot {
        externalImports = append(externalImports, pkg.ImportPath)
      }
    }

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

  // Create temporary directory to simulate empty GOPATH. `go get` will download
  // the dependencies into this path where we later copy them from.
  tmp, err := ioutil.TempDir("", "cedric-gopath-")
  handleErr(err)

  outputTemplate.Execute(os.Stdout, templateInput{
    Cwd: cwd,
    Tmp: tmp,
    Packages: externalImports,
    AddSubmodules: addSubmodules,
  })
}

func handleErr(err error) {
  if err == nil {
    return
  }

  fmt.Fprintf(os.Stderr, "Internal error occured:\n\t%s\n", err)

  os.Exit(1)
}
