package main

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/huffduff/weave"
)

func main() {
	// Define command line flags
	cmd := &cli.Command{
		Commands: []*cli.Command{
			{
				Name:  "schema",
				Usage: "Generate Weaviate schema definitions from Go struct definitions",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "pretty",
						Aliases: []string{"p"},
						Usage:   "Pretty-print the JSON output",
					},
					&cli.StringFlag{
						Name:    "output",
						Aliases: []string{"o"},
						Usage:   "Output file for the generated schema",
					},
				},

				Action: generateSchema,
			},
			{
				Name:  "crud",
				Usage: "Generate Weaviate CRUD operations for Weaviate objects",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "output",
						Aliases: []string{"o"},
						Usage:   "Output directory for the generated package",
					},
					&cli.BoolFlag{
						Name:    "include-types",
						Aliases: []string{"t"},
						Usage:   "Include useful helper types",
					},
				},
				Action: generateCrud,
			},
		}}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		panic(err)
	}
}

func generateSchema(ctx context.Context, c *cli.Command) error {
	srcDir := c.Args().First()
	if srcDir == "" {
		return fmt.Errorf("source directory is required")
	}

	output := c.String("output")

	pretty := c.Bool("pretty")

	// Generate the schema
	schema, err := weave.GenerateWeaviateSchema(srcDir)
	if err != nil {
		return fmt.Errorf("error generating schema: %v", err)
	}

	// Marshal to JSON
	jsonOutput, err := schema.ToJSON(pretty)
	if err != nil {
		return fmt.Errorf("error marshaling schema to JSON: %v", err)
	}

	// Output the schema
	if output == "" {
		// Output to stdout
		fmt.Println(string(jsonOutput))
	} else {
		// Output to file
		err = os.WriteFile(output, jsonOutput, 0644)
		if err != nil {
			return fmt.Errorf("error writing to output file: %v", err)
		}
		fmt.Printf("Schema successfully written to %s\n", output)
	}

	return nil
}

func generateCrud(ctx context.Context, c *cli.Command) error {
	srcDir := c.Args().First()
	if srcDir == "" {
		return fmt.Errorf("source directory is required")
	}

	output := c.String("output")
	if output == "" {
		output = srcDir
	}

	includeTypes := c.Bool("include-types")

	schema, err := weave.GenerateWeaviateSchema(srcDir)
	if err != nil {
		return fmt.Errorf("error generating schema: %v", err)
	}

	packageName, err := weave.GenerateCRUDCode(schema, output)
	if err != nil {
		return fmt.Errorf("error generating crud code: %v", err)
	}

	if includeTypes {
		err = weave.GenerateTypes(packageName, output)
		if err != nil {
			return fmt.Errorf("error generating types: %v", err)
		}
	}
	return nil
}
