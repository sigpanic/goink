package main

import (
	"fmt"

	"github.com/sigpanic/goink/internal/version"
)

func main() {
	fmt.Print(version.BuildHash())
}
