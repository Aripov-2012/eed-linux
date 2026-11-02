package customization

import "fmt"

// ValidateSelection verifies a user selection does not contain unsupported or empty values.
func ValidateSelection(state DesiredState) error {
	if err := Validate(state); err != nil {
		return err
	}
	if len(SelectedComponents(state)) == 0 && state.Desktop == "desktop.none" {
		return nil
	}
	return nil
}

// ValidateUserSelection exposes a valid state error message for planning logic.
func ValidateUserSelection(state DesiredState) error {
	if err := ValidateSelection(state); err != nil {
		return fmt.Errorf("invalid selection: %w", err)
	}
	return nil
}
