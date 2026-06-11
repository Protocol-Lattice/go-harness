package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

func main() {
	root := flag.String("root", ".", "filesystem root")
	flag.Parse()

	args := flag.Args()

	if len(args) > 0 && args[0] == "list-tools" {
		tools := []map[string]any{
			{
				"name":        "fs.list",
				"description": "List files and directories under the configured root",
			},
			{
				"name":        "fs.read",
				"description": "Read a file under the configured root",
			},
			{
				"name":        "fs.write",
				"description": "Write a file under the configured root",
			},
			{
				"name":        "fs.mkdir",
				"description": "Create a directory under the configured root",
			},
			{
				"name":        "fs.remove",
				"description": "Remove a file or directory under the configured root",
			},
			{
				"name":        "fs.stat",
				"description": "Get file or directory metadata under the configured root",
			},
			{
				"name":        "fs.rename",
				"description": "Rename a file or directory under the configured root",
			},
			{
				"name":        "fs.copy",
				"description": "Copy a file or directory under the configured root",
			},
			{
				"name":        "fs.exists",
				"description": "Check if a file or directory exists under the configured root",
			},
		}

		if err := json.NewEncoder(os.Stdout).Encode(tools); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	_ = root

	// existing tool-call handling here
}
