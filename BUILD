subinclude("///go//build_defs:go")

go_library(
    name = "ar",
    srcs = glob(
        ["*.go"],
        exclude = ["*_test.go"],
    ),
)

go_test(
    name = "ar_test",
    srcs = glob(["*_test.go"]),
    data = glob(["test_data/*"]),
    deps = [
        "///third_party/go/github.com_stretchr_testify//assert",
        "///third_party/go/github.com_stretchr_testify//require",
        ":ar",
    ],
)
