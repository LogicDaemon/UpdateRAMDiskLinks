package main

import (
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func processEnvBlock(envNode *yaml.Node) error {
	if envNode.Kind != yaml.MappingNode {
		return nil
	}

	type envDef struct {
		key      string
		valRaw   string
		resolved bool
		val      string
	}

	var defs []*envDef
	for j := 0; j < len(envNode.Content); j += 2 {
		k := envNode.Content[j].Value
		v := envNode.Content[j+1].Value
		defs = append(defs, &envDef{key: k, valRaw: v})
	}

	progress := true
	for progress {
		progress = false
		for _, d := range defs {
			if d.resolved {
				continue
			}
			exp, err := expandEnv(d.valRaw)
			if err == nil {
				d.val = exp
				d.resolved = true
				progress = true

				k := d.key
				checkExists := false
				if strings.HasPrefix(k, "?") {
					checkExists = true
					k = k[1:]
				}

				if checkExists {
					if existing, ok := getEnv(k); !ok || existing == "" {
						setEnv(k, exp)
						os.Setenv(k, exp) // Update OS environment for executed processes
					}
				} else {
					setEnv(k, exp)
					os.Setenv(k, exp) // Update OS environment for executed processes
				}
			}
		}
	}

	for _, d := range defs {
		if !d.resolved {
			log.Printf("Warning: failed to resolve env var %s", d.key)
		}
	}
	return nil
}

func setupLog(logPath string) {
	if logPath == "" {
		return
	}
	exp, err := expandEnv(logPath)
	if err != nil {
		log.Printf("Warning: failed to expand log path '%s': %v", logPath, err)
		return
	}
	if !filepath.IsAbs(exp) {
		if configDir != "" {
			exp = filepath.Join(configDir, exp)
		}
	}

	// Create directory for the log file if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(exp), os.ModePerm); err != nil {
		log.Printf("Warning: failed to create log directory '%s': %v", filepath.Dir(exp), err)
	}

	f, err := os.OpenFile(exp, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Printf("Warning: failed to open log file '%s': %v", exp, err)
		return
	}

	mw := io.MultiWriter(os.Stdout, f)
	log.SetOutput(mw)
}

func runShellCommands(node *yaml.Node) {
	if node.Kind != yaml.SequenceNode {
		log.Printf("Warning: Expected sequence for shell commands")
		return
	}
	for _, n := range node.Content {
		cmdStr, err := expandEnv(n.Value)
		if err != nil {
			log.Printf("Error expanding command '%s': %v", n.Value, err)
			continue
		}

		log.Printf("Executing: %s", cmdStr)

		cmd := exec.Command("cmd", "/C", cmdStr)
		cmd.Stdout = log.Writer()
		cmd.Stderr = log.Writer()

		if err := cmd.Run(); err != nil {
			log.Printf("Command '%s' failed: %v", cmdStr, err)
		} else {
			log.Printf("Command '%s' completed successfully", cmdStr)
		}
	}
}
