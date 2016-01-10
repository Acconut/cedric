**cedric(1)** is a vendoring tool for Go built with simplicity in mind. Its
only purpose and functionality is to read dependencies from a package,
download, and copy them into the `vendor/` directory. This allows authors to
use Go's vendoring system for importing external packages.

*cedric's* functionality includes:

* No additional configuration file for version locking or similar used
* No requirement on the Go vendor experiment by itself
* Scan packages and its subdirectories recursively
* Automatically add Git repositories as submodules
* Flat dependency tree

# Installation

```
go get -u github.com/Acconut/cedric
```

# Usage

**cedric(1)** will not download the dependencies on its own. Instead it
outputs a Bash script which, when executed, will download and copy the external
packages into the vendor directory. By default, it will scan the current
package recursively (excluding vendor/ and other directories such as .git/).

```bash
$ bash -c "$(cedric)"
```

# Example

```bash
$ cat <<EOF > main.go
package main

import (
        "github.com/fatih/color"
)

func main() {
        color.Cyan("Hello World!")
}
EOF

$ git init

$ bash -c "$(cedric)"

$ go run main.go
# Hello World!

$ tree -d
#.
#└── vendor
#    └── github.com
#        ├── fatih
#        │   └── color
#        └── mattn
#            ├── go-colorable
#            │   └── _example
#            └── go-isatty
#                └── _example

$ git submodules status
# 9aae6aaa22315390f03959adca2c4d395b02fcef vendor/github.com/fatih/color (v0.1-12-g9aae6aa)
# 3dac7b4f76f6e17fb39b768b89e3783d16e237fe vendor/github.com/mattn/go-colorable (heads/master)
# 56b76bdf51f7708750eac80fa38b952bb9f32639 vendor/github.com/mattn/go-isatty (heads/master)
```

# License

MIT
