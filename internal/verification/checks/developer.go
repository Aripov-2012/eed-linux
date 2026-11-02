package checks

import "eed/internal/verification"

func registerDeveloperChecks(registry *verification.Registry) error {
	return registerPackageChecks(registry)
}
