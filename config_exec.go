package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
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
		logPath, ok := getEnv("LOG")
		if !ok || logPath == "" {
			return
		}
	} else {
		exp, err := expandEnv(logPath)
		if err != nil {
			log.Printf("Warning: failed to expand log path '%s': %v", logPath, err)
			return
		}
		logPath = exp
	}

	if !filepath.IsAbs(logPath) {
		logPath = filepath.Join(configDir, logPath)
	}

	setEnv("LOG", logPath)
	os.Setenv("LOG", logPath)

	// Create directory for the log file if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(logPath), os.ModePerm); err != nil {
		log.Printf("Warning: failed to create log directory '%s': %v", filepath.Dir(logPath), err)
	}

	f, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Printf("Warning: failed to open log file '%s': %v", logPath, err)
		return
	}

	mw := io.MultiWriter(os.Stderr, f)
	log.SetOutput(mw)
}

func runLoggedCommand(name string, args []string, workingDir string, display string) error {
	if display == "" {
		display = windows.ComposeCommandLine(append([]string{name}, args...))
	}

	log.Printf("Executing: %s", display)

	cmd := exec.Command(name, args...)
	cmd.Stdout = log.Writer()
	cmd.Stderr = log.Writer()
	cmd.Env = os.Environ()
	if workingDir != "" {
		cmd.Dir = workingDir
	}

	err := cmd.Run()
	if err == nil {
		log.Printf("Command '%s' exited with code 0", display)
		return nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		log.Printf("Command '%s' exited with code %d", display, exitErr.ExitCode())
		return err
	}

	log.Printf("Command '%s' failed before returning an exit code: %v", display, err)
	return err
}

func runCommandLine(commandLine string, workingDir string) error {
	args, err := windows.DecomposeCommandLine(commandLine)
	if err != nil {
		return fmt.Errorf("parse command line %q: %w", commandLine, err)
	}
	if len(args) == 0 {
		return fmt.Errorf("empty command line")
	}

	return runLoggedCommand(args[0], args[1:], workingDir, commandLine)
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

		if err := runCommandLine(cmdStr, ""); err != nil {
			log.Printf("Command '%s' failed: %v", cmdStr, err)
		}
	}
}
