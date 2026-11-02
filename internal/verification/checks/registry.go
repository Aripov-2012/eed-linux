package checks

import "eed/internal/verification"

func DefaultRegistry() (*verification.Registry, error) {
	registry := verification.NewRegistry()
	register := []func(*verification.Registry) error{
		registerSystemChecks,
		registerDeveloperChecks,
		registerDesktopChecks,
		registerHardwareChecks,
		registerServiceChecks,
		registerConfigurationChecks,
		registerRecoveryChecks,
	}
	for _, add := range register {
		if err := add(registry); err != nil {
			return nil, err
		}
	}
	return registry, nil
}
