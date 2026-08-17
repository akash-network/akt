package cli

import "fmt"

func requireQueryResponse[T any](operation string, response *T) error {
	if response == nil {
		return fmt.Errorf("%s query returned malformed node response: missing response", operation)
	}

	return nil
}

func requireQueryField[T any](operation, field string, value *T) error {
	if value == nil {
		return fmt.Errorf("%s query returned malformed node response: missing %s", operation, field)
	}

	return nil
}
