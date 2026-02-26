package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"integrationtests/docker"
	"integrationtests/scenario"
)

const defaultGateway = "localhost:10000"

// Default test user built into MyAuth for integration tests (login scenarios use it when no credentials provided).
const (
	defaultTestUsername = "TestUser"
	defaultTestPassword = "TestPassword"
)

func main() {
	list := flag.Bool("list", false, "list available scenarios and exit")
	scenarioName := flag.String("scenario", "", "scenario to run (or pass as positional arg)")
	gateway := flag.String("gateway", "", "gateway address (default: localhost:10000 or GATEWAY_ADDR env)")
	username := flag.String("username", "", "username for Login (default: TEST_USERNAME env or built-in test user)")
	password := flag.String("password", "", "password for Login (default: TEST_PASSWORD env or built-in test user)")
	composeFile := flag.String("compose-file", "", "path to docker-compose.yml (default: COMPOSE_FILE env or ../../docker-compose.yml)")
	flag.Parse()

	if *gateway == "" {
		*gateway = os.Getenv("GATEWAY_ADDR")
	}
	if *gateway == "" {
		*gateway = defaultGateway
	}
	if *username == "" {
		*username = os.Getenv("TEST_USERNAME")
	}
	if *username == "" {
		*username = defaultTestUsername
	}
	if *password == "" {
		*password = os.Getenv("TEST_PASSWORD")
	}
	if *password == "" {
		*password = defaultTestPassword
	}

	if *list {
		all := scenario.All()
		for name := range all {
			fmt.Println(name)
		}
		os.Exit(0)
	}

	// Determine compose file path
	composePath := *composeFile
	if composePath == "" {
		composePath = os.Getenv("COMPOSE_FILE")
	}
	if composePath == "" {
		composePath = docker.DefaultComposeFile
	}

	// Setup docker-compose environment before running scenario
	if err := docker.SetupEnvironment(composePath); err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to setup docker-compose environment: %v\n", err)
		os.Exit(1)
	}

	name := *scenarioName
	if name == "" {
		args := flag.Args()
		if len(args) > 0 {
			name = args[0]
		}
	}
	if name == "" {
		fmt.Fprintln(os.Stderr, "usage: integrationtests [--list] [--scenario=NAME] [--gateway=ADDR] [--username=U] [--password=P] [--compose-file=PATH] [scenario_name]")
		fmt.Fprintln(os.Stderr, "  use --list to list scenarios")
		os.Exit(2)
	}

	cfg := &scenario.Config{
		GatewayAddr: *gateway,
		Username:    *username,
		Password:    *password,
		ComposePath: composePath,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Run scenario and capture result
	err := scenario.Run(name, ctx, cfg)

	// Output scenario result
	fmt.Println("\n=== Scenario Result ===")
	fmt.Printf("Scenario: %s\n", name)

	if err != nil {
		var unknown *scenario.UnknownScenarioError
		if errors.As(err, &unknown) {
			fmt.Printf("Status: FAILED\n")
			fmt.Printf("Error: %v\n", err)
			all := scenario.All()
			if len(all) > 0 {
				names := make([]string, 0, len(all))
				for n := range all {
					names = append(names, n)
				}
				fmt.Fprintf(os.Stderr, "\navailable scenarios: %s\n", strings.Join(names, ", "))
			}
			fmt.Println("=====================")
			os.Exit(2)
		}
		fmt.Printf("Status: FAILED\n")
		fmt.Printf("Error: %v\n", err)
		fmt.Println("=====================")
		os.Exit(1)
	}

	fmt.Printf("Status: PASSED\n")
	fmt.Println("=====================")
	os.Exit(0)
}
