package weave

import "path/filepath"

func GenerateTypes(packageName string, outputDir string) error {
	templateData := TemplateData[struct{}]{
		PackageName: packageName,
	}
	return generateFromTemplate("types", templateData, filepath.Join(outputDir, "weave_types.go"))
}
