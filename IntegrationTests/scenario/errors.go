package scenario

import "fmt"

// UnknownScenarioError is returned when the requested scenario name is not registered.
type UnknownScenarioError struct {
	Name string
}

func (e *UnknownScenarioError) Error() string {
	return fmt.Sprintf("unknown scenario: %s", e.Name)
}
