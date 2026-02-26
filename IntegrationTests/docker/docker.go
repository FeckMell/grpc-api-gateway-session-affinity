package docker

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	// DefaultComposeFile is the default path to docker-compose.yml relative to project root
	DefaultComposeFile = "../docker-compose.yml"
	// ContainerStartupTimeout is the maximum time to wait for containers to start
	ContainerStartupTimeout = 60 * time.Second
	// PostStartupDelay is the delay after containers are ready before starting tests.
	PostStartupDelay = 5 * time.Second
	// StatusCheckInterval is the interval between status checks
	StatusCheckInterval = 2 * time.Second
)

// SetupEnvironment prepares the docker-compose environment by running down and up.
// It waits for all containers to be ready and then waits an additional 5 seconds.
// Returns an error if containers fail to start.
func SetupEnvironment(composePath string) error {
	if composePath == "" {
		composePath = DefaultComposeFile
	}

	// Resolve absolute path
	absPath, err := filepath.Abs(composePath)
	if err != nil {
		return fmt.Errorf("resolve compose file path: %w", err)
	}

	// Check if file exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return fmt.Errorf("docker-compose.yml not found at %s", absPath)
	}

	// Get directory containing docker-compose.yml for working directory
	composeDir := filepath.Dir(absPath)

	fmt.Fprintf(os.Stderr, "=== Setting up docker-compose environment ===\n")
	fmt.Fprintf(os.Stderr, "Compose file: %s\n", absPath)
	fmt.Fprintf(os.Stderr, "Working directory: %s\n\n", composeDir)

	// Step 1: docker-compose down
	fmt.Fprintf(os.Stderr, "Running: docker-compose down\n")
	if err := runComposeCommand(composeDir, "down"); err != nil {
		// Log warning but continue - it's okay if nothing was running
		fmt.Fprintf(os.Stderr, "Warning: docker-compose down failed (this is okay if nothing was running): %v\n", err)
	}

	// Step 2: docker-compose up -d
	fmt.Fprintf(os.Stderr, "\nRunning: docker-compose up -d\n")
	if err := runComposeUp(composeDir); err != nil {
		return fmt.Errorf("docker-compose up failed: %w", err)
	}

	// Step 3: Wait for containers to be ready
	fmt.Fprintf(os.Stderr, "\nWaiting for containers to be ready...\n")
	if err := waitForContainersReady(composeDir); err != nil {
		return fmt.Errorf("containers failed to start: %w", err)
	}

	// Step 4: Wait additional 5 seconds as specified
	fmt.Fprintf(os.Stderr, "\nAll containers are ready. Waiting %v before starting tests...\n", PostStartupDelay)
	time.Sleep(PostStartupDelay)

	fmt.Fprintf(os.Stderr, "=== Docker-compose environment ready ===\n\n")
	return nil
}

// runComposeCommand runs a docker-compose command with output redirected to stderr
func runComposeCommand(workDir string, args ...string) error {
	cmd := exec.Command("docker-compose", args...)
	cmd.Dir = workDir
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// StopService stops a docker-compose service by name (e.g. "myservice").
// workDir must be the directory containing docker-compose.yml.
func StopService(workDir string, serviceName string) error {
	return runComposeCommand(workDir, "stop", serviceName)
}

// StartService starts a docker-compose service by name (e.g. "myservice").
// workDir must be the directory containing docker-compose.yml.
func StartService(workDir string, serviceName string) error {
	return runComposeCommand(workDir, "start", serviceName)
}

// ServiceContainerIDs returns all container IDs for a docker-compose service.
func ServiceContainerIDs(workDir string, serviceName string) ([]string, error) {
	cmd := exec.Command("docker-compose", "ps", "-q", serviceName)
	cmd.Dir = workDir
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker-compose ps -q %s failed: %w", serviceName, err)
	}

	var ids []string
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		id := strings.TrimSpace(scanner.Text())
		if id == "" {
			continue
		}
		ids = append(ids, id)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parse container ids: %w", err)
	}
	return ids, nil
}

// StopContainer stops a single container by container ID (SIGTERM, graceful).
func StopContainer(containerID string) error {
	cmd := exec.Command("docker", "stop", containerID)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// KillContainer immediately kills a container (SIGKILL). Use when the test requires the connection to drop without waiting for graceful shutdown.
func KillContainer(containerID string) error {
	cmd := exec.Command("docker", "kill", containerID)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// StartContainer starts a single container by container ID.
func StartContainer(containerID string) error {
	cmd := exec.Command("docker", "start", containerID)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ContainerHostname returns the hostname of a container (e.g. used as instance id / pod name).
// Uses docker inspect --format '{{.Config.Hostname}}'; if unset, Docker uses the container ID.
func ContainerHostname(containerID string) (string, error) {
	cmd := exec.Command("docker", "inspect", "--format", "{{.Config.Hostname}}", containerID)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("docker inspect hostname %s: %w", containerID, err)
	}
	return strings.TrimSpace(string(output)), nil
}

// runComposeUp runs docker-compose up -d with output redirected to stderr
func runComposeUp(workDir string) error {
	cmd := exec.Command("docker-compose", "up", "-d")
	cmd.Dir = workDir
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// waitForContainersReady waits for all containers to be in "Up" state
func waitForContainersReady(workDir string) error {
	ctx, cancel := context.WithTimeout(context.Background(), ContainerStartupTimeout)
	defer cancel()

	ticker := time.NewTicker(StatusCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for containers to start (waited %v)", ContainerStartupTimeout)
		case <-ticker.C:
			status, err := checkContainersStatus(workDir)
			if err != nil {
				return fmt.Errorf("failed to check container status: %w", err)
			}

			if status.allUp {
				return nil
			}

			if status.hasFailed {
				return fmt.Errorf("one or more containers failed to start: %s", status.failedContainers)
			}

			// Continue waiting
		}
	}
}

// containerStatus represents the status of containers
type containerStatus struct {
	allUp            bool
	hasFailed        bool
	failedContainers string
}

// containerInfo represents a single container's status from docker-compose ps
type containerInfo struct {
	Name   string `json:"Name"`
	State  string `json:"State"`
	Status string `json:"Status"`
}

// checkContainersStatus checks the status of all containers using docker-compose ps
func checkContainersStatus(workDir string) (*containerStatus, error) {
	cmd := exec.Command("docker-compose", "ps", "--format", "json")
	cmd.Dir = workDir
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker-compose ps failed: %w", err)
	}

	status := &containerStatus{
		allUp:     true,
		hasFailed: false,
	}

	// Parse JSON output line by line (each line is a separate JSON object)
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	var failedNames []string
	var containerCount int

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var container containerInfo
		if err := json.Unmarshal([]byte(line), &container); err != nil {
			// If JSON parsing fails, try to continue with next line
			continue
		}

		containerCount++

		// Check container state
		state := strings.ToLower(container.State)
		statusValue := strings.ToLower(container.Status)

		// Container is running if state is "running"
		if state != "running" {
			status.allUp = false

			// Check if container has failed (exited, restarting, etc.)
			if state == "exited" || state == "dead" ||
				strings.Contains(statusValue, "exit") ||
				strings.Contains(statusValue, "restarting") {
				status.hasFailed = true
				failedNames = append(failedNames, container.Name)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to parse docker-compose ps output: %w", err)
	}

	if len(failedNames) > 0 {
		status.failedContainers = strings.Join(failedNames, ", ")
	}

	// If no containers found, consider it as not ready
	if containerCount == 0 {
		// Check if there are any containers at all
		cmd := exec.Command("docker-compose", "ps", "-q")
		cmd.Dir = workDir
		output, err := cmd.Output()
		if err == nil && len(strings.TrimSpace(string(output))) == 0 {
			return nil, fmt.Errorf("no containers found")
		}
		// If containers exist but ps --format json returned nothing, they might still be starting
		status.allUp = false
	}

	return status, nil
}
