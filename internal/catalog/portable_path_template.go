package catalog

import (
	"errors"
	"strings"
)

var allowedSavePathVariables = map[string]bool{
	"edition.id": true, "edition.save_namespace": true, "edition.serial": true,
	"edition.product_code": true, "edition.title_id": true,
	"edition.title_id_high": true, "edition.title_id_low": true,
	"platform.id": true, "rom.stem": true,
	"driver.id": true, "driver.user_dir": true,
	"device.id": true, "device.target": true,
	"device.config_dir": true, "device.save_dir": true, "device.core_dir": true, "device.emulator_dir": true,
}

// ValidateSavePathTemplate accepts only inert path templates. Host roots must
// come from an explicitly paired device configuration; environment variables,
// shell expressions and conditionals are not part of the format.
func ValidateSavePathTemplate(value string) error {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" || len(value) > 1024 || strings.ContainsAny(value, "\x00\r\n") {
		return errors.New("invalid local save path template")
	}
	if strings.HasPrefix(value, "/") || (len(value) >= 3 && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) && value[1] == ':' && value[2] == '/') {
		return errors.New("local save path templates must begin with an authorized root variable or a relative path")
	}
	for _, part := range strings.Split(value, "/") {
		if part == ".." {
			return errors.New("local save path templates must not contain parent traversal")
		}
	}
	cleaned := launchVariablePattern.ReplaceAllStringFunc(value, func(match string) string {
		name := strings.TrimSuffix(strings.TrimPrefix(match, "{{"), "}}")
		if allowedSavePathVariables[name] {
			return ""
		}
		return match
	})
	if strings.Contains(cleaned, "{{") || strings.Contains(cleaned, "}}") || strings.ContainsAny(cleaned, "{}") {
		return errors.New("local save path contains an unknown or malformed template variable")
	}
	return nil
}
