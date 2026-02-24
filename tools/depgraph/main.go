package main

// 临时工具：输出 clashgo 包间依赖图
// 运行：go run tools/depgraph/main.go

import (
	"fmt"
	"go/build"
	"sort"
	"strings"
)

func main() {
	pkgs := []string{
		"clashgo/internal/utils",
		"clashgo/internal/config",
		"clashgo/internal/mihomo",
		"clashgo/internal/enhance",
		"clashgo/internal/core",
		"clashgo/internal/proxy",
		"clashgo/internal/backup",
		"clashgo/internal/tray",
		"clashgo/internal/hotkey",
		"clashgo/internal/updater",
		"clashgo/api",
		"clashgo",
	}

	fmt.Println("=== ClashGo Package Dependency Graph ===")
	fmt.Println()

	ctx := build.Default
	ctx.GOPATH = ""

	for _, pkgPath := range pkgs {
		pkg, err := ctx.Import(pkgPath, ".", 0)
		if err != nil {
			fmt.Printf("%-40s  [error: %v]\n", pkgPath, err)
			continue
		}

		var internal []string
		for _, imp := range pkg.Imports {
			if strings.HasPrefix(imp, "clashgo/") {
				internal = append(internal, strings.TrimPrefix(imp, "clashgo/"))
			}
		}
		sort.Strings(internal)

		short := strings.TrimPrefix(pkgPath, "clashgo/")
		if len(internal) == 0 {
			fmt.Printf("%-35s  (no internal deps)\n", short)
		} else {
			fmt.Printf("%-35s  -> %s\n", short, strings.Join(internal, ", "))
		}
	}

	fmt.Println()
	fmt.Println("Layer order (bottom -> top):")
	fmt.Println("  L0: internal/utils     (foundation, no internal deps)")
	fmt.Println("  L1: internal/config    (types + manager, uses utils)")
	fmt.Println("  L2: internal/mihomo    (HTTP client, no internal deps)")
	fmt.Println("  L3: internal/enhance   (pipeline, uses config+utils)")
	fmt.Println("  L4: internal/core      (lifecycle, uses config+enhance+mihomo+utils)")
	fmt.Println("  L5: internal/proxy     (sysproxy, uses config+utils)")
	fmt.Println("  L6: internal/backup    (backup, uses config+utils)")
	fmt.Println("  L7: internal/tray      (menu, uses core+proxy+config+utils)")
	fmt.Println("  L8: internal/hotkey    (hotkeys, uses config+utils)")
	fmt.Println("  L9: internal/updater   (autoupdate, uses config+utils)")
	fmt.Println("  L10: api/              (Wails bindings, uses all internals)")
	fmt.Println("  L11: clashgo (main)    (Wails app, uses api+all internals)")
}
