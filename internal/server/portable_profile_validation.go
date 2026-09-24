package server

// Read-only validation for legacy portable ROM metadata. No device execution.
import (
	"errors"
	"fmt"
	"kino/internal/catalog"
	"path/filepath"
	"regexp"
	"strings"
)

type Profile struct {
	ID                string           `json:"id,omitempty"`
	Name              string           `json:"name"`
	Frontend          string           `json:"frontend"`
	Target            string           `json:"target"`
	DeviceProfileID   string           `json:"device_profile_id,omitempty"`
	FrontendAdapterID string           `json:"frontend_adapter_id,omitempty"`
	Locale            string           `json:"locale"`
	FileMode          string           `json:"file_mode"`
	OutputSlug        string           `json:"output_slug,omitempty"`
	Enabled           bool             `json:"enabled"`
	Templates         []ConfigTemplate `json:"templates,omitempty"`
}

type ConfigTemplate struct {
	Name       string `json:"name"`
	Scope      string `json:"scope"`
	OutputPath string `json:"output_path"`
	Body       string `json:"body"`
}

var placeholderPattern = regexp.MustCompile(`\{\{\s*([a-z][a-z0-9_.]*)\s*\}\}`)
var anyTemplateAction = regexp.MustCompile(`\{\{[^}]*\}\}`)

var allowedTemplateVariables = map[string]bool{
	"profile.name": true, "profile.frontend": true, "profile.target": true, "profile.locale": true, "profile.file_mode": true,
	"platform.id": true, "game.id": true, "game.title": true, "edition.id": true, "edition.title": true, "edition.type": true,
	"edition.save_namespace": true, "edition.serial": true, "edition.product_code": true, "edition.title_id": true,
	"rom.path": true, "rom.source_path": true, "rom.stem": true,
	"device.id": true, "device.target": true, "device.config_dir": true, "device.save_dir": true, "device.rom_dir": true, "device.core_dir": true, "device.emulator_dir": true,
	"driver.id": true, "driver.family": true, "core.id": true, "core.library": true,
	"launch.android_package": true, "launch.android_activity": true, "launch.arguments_json": true, "launch.executable_hints_json": true,
}

func newPackageProfileToBundler(profile catalog.NewPackageProfile) Profile {
	templates := make([]ConfigTemplate, len(profile.Templates))
	for index, template := range profile.Templates {
		templates[index] = ConfigTemplate{Name: template.Name, Scope: template.Scope, OutputPath: template.OutputPath, Body: template.Body}
	}
	return Profile{Name: profile.Name, Frontend: profile.Frontend, Target: profile.Target, DeviceProfileID: profile.DeviceProfileID, FrontendAdapterID: profile.FrontendAdapterID, Locale: profile.Locale, FileMode: profile.FileMode, OutputSlug: profile.OutputSlug, Enabled: profile.Enabled == nil || *profile.Enabled, Templates: templates}
}

func validatePackageProfile(in catalog.NewPackageProfile) error {
	_, err := normalizeProfile(newPackageProfileToBundler(in))
	return err
}
func normalizeProfile(profile Profile) (Profile, error) {
	profile.Name = strings.TrimSpace(profile.Name)
	profile.Frontend = strings.ToLower(strings.TrimSpace(profile.Frontend))
	profile.Target = strings.ToLower(strings.TrimSpace(profile.Target))
	profile.Locale = strings.TrimSpace(profile.Locale)
	profile.FileMode = strings.ToLower(strings.TrimSpace(profile.FileMode))
	profile.DeviceProfileID = strings.TrimSpace(profile.DeviceProfileID)
	profile.FrontendAdapterID = strings.TrimSpace(profile.FrontendAdapterID)
	if profile.Name == "" {
		return profile, errors.New("profile name is required")
	}
	if profile.Frontend != "pegasus" && profile.Frontend != "es-de" {
		return profile, errors.New("frontend must be pegasus or es-de")
	}
	if profile.FileMode == "" {
		profile.FileMode = "copy"
	}
	if profile.FileMode != "copy" && profile.FileMode != "hardlink" && profile.FileMode != "reference" {
		return profile, errors.New("file_mode must be copy, hardlink, or reference")
	}
	if profile.Locale == "" {
		profile.Locale = "zh-CN"
	}
	for index := range profile.Templates {
		profile.Templates[index].Name = strings.TrimSpace(profile.Templates[index].Name)
		profile.Templates[index].Scope = strings.ToLower(strings.TrimSpace(profile.Templates[index].Scope))
		profile.Templates[index].OutputPath = filepath.ToSlash(strings.TrimSpace(profile.Templates[index].OutputPath))
		if err := validateConfigTemplate(profile.Templates[index]); err != nil {
			return profile, fmt.Errorf("template %d: %w", index, err)
		}
	}
	return profile, nil
}

func validateConfigTemplate(template ConfigTemplate) error {
	if template.Name == "" || template.OutputPath == "" {
		return errors.New("name and output_path are required")
	}
	if template.Scope != "package" && template.Scope != "platform" && template.Scope != "edition" {
		return errors.New("scope must be package, platform, or edition")
	}
	if len(template.Body) > 64*1024 || len(template.OutputPath) > 512 {
		return errors.New("template exceeds the safe size limit")
	}
	if strings.ContainsRune(template.Body, 0) || strings.ContainsRune(template.OutputPath, 0) {
		return errors.New("template must not contain NUL")
	}
	if strings.Contains(template.OutputPath, "\\") || strings.Contains(template.OutputPath, ":") {
		return errors.New("output_path must use a portable relative path")
	}
	if _, err := cleanRelative(template.OutputPath); err != nil {
		return fmt.Errorf("output_path: %w", err)
	}
	allowedExtensions := map[string]bool{".json": true, ".xml": true, ".ini": true, ".cfg": true, ".conf": true, ".toml": true, ".yaml": true, ".yml": true, ".txt": true, ".properties": true, ".opt": true}
	if extension := strings.ToLower(filepath.Ext(template.OutputPath)); !allowedExtensions[extension] {
		return fmt.Errorf("output_path extension %q is not an allowed configuration type", extension)
	}
	for _, value := range []string{template.Body, template.OutputPath} {
		for _, match := range placeholderPattern.FindAllStringSubmatch(value, -1) {
			if !allowedTemplateVariables[match[1]] {
				return fmt.Errorf("unknown template variable %s", match[1])
			}
			if template.Scope == "package" && !strings.HasPrefix(match[1], "profile.") && !strings.HasPrefix(match[1], "device.") {
				return fmt.Errorf("variable %s is unavailable in package scope", match[1])
			}
			if template.Scope == "platform" && !strings.HasPrefix(match[1], "profile.") && !strings.HasPrefix(match[1], "device.") && match[1] != "platform.id" {
				return fmt.Errorf("variable %s is unavailable in platform scope", match[1])
			}
		}
		remaining := placeholderPattern.ReplaceAllString(value, "")
		if anyTemplateAction.MatchString(remaining) {
			return errors.New("only simple {{variable.name}} placeholders are allowed")
		}
	}
	return nil
}

func cleanRelative(value string) (string, error) {
	value = filepath.Clean(filepath.FromSlash(strings.TrimSpace(value)))
	if value == "." || value == "" || filepath.IsAbs(value) || value == ".." || strings.HasPrefix(value, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("artifact path %q is not a safe relative path", value)
	}
	return value, nil
}
