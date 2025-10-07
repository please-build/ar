package(default_visibility = ["PUBLIC"])

subinclude("///go//build_defs:go")

go_toolchain(
    name = "toolchain",
    install_std = False,
    version = "1.24.7",
)

go_stdlib(
    name = "std",
    tags = [
        "netgo",
        "osusergo",
    ],
)

go_repo(
    licences = ["MIT"],
    module = "github.com/stretchr/testify",
    version = "v1.11.1",
)

go_repo(
    licences = ["ISC"],
    module = "github.com/davecgh/go-spew",
    version = "v1.1.1",
)

go_repo(
    licences = ["BSD-3-Clause"],
    module = "github.com/pmezard/go-difflib",
    version = "v1.0.0",
)

go_repo(
    licences = [
        "MIT",
        "Apache-2.0",
    ],
    module = "gopkg.in/yaml.v3",
    version = "v3.0.1",
)
