package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v3"
)

type Config struct {
	TemplateId  string                 `yaml:"template_id"`
	Base        map[string]interface{} `yaml:"base"`
	Pages       map[string]interface{} `yaml:"pages"`
	Collections map[string]interface{} `yaml:"collections"`
}

type GitHubTreeItem struct {
	Path string `json:"path"`
	Type string `json:"type"`
}

type GitHubTree struct {
	Tree []GitHubTreeItem `json:"tree"`
}

func main() {
	// Custom usage message for --help
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [options]\n\n", os.Args[0])
		fmt.Println("A portfolio generator that uses a template and a configuration file to build a static website.")
		fmt.Println("Available commands:")
		fmt.Println("  --template <template-id>  : Downloads the configuration for a specific template. Use this first!")
		fmt.Println("  --config <file-path>      : (Optional) Specifies the path to your configuration file (default: config.yml).")
		fmt.Println("  --output-dir <dir-path>   : (Optional) Sets the output directory for the generated site (default: .).")
		fmt.Println("  --help                    : Shows this help message.")
		fmt.Println("  --config                    : (Optional) Path to config file (default: .)")
		fmt.Println("\nExample usage:")
		fmt.Println("  To download a template configuration: ./portfolio-builder --template=0001")
		fmt.Println("  To generate your portfolio: ./portfolio-builder")
	}

	templateId, configFilePath, outputDir := getArgs()
	configUrl := fmt.Sprintf("https://raw.githubusercontent.com/saksham-ghimire/portfolio-builder/main/templates/%s/config.yml", templateId)

	if templateId != "" {
		log.Println("Fetching template configuration for id:", templateId)
		downloadFile(configUrl, "config.yml")
		log.Println("Successfully fetched the configuration, please update 'config.yml' as needed, and then execute the program without --template to generate your portfolio.")
		return
	}

	var config = readConfig(configFilePath)
	schemaUrl := fmt.Sprintf("https://raw.githubusercontent.com/saksham-ghimire/portfolio-builder/main/templates/%s/schema.json", config.TemplateId)
	validateConfig(schemaUrl, config)

	templateDir := downloadTemplateFromGitHub(config.TemplateId)
	defer os.RemoveAll(templateDir)

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatalf("Error creating output directory. Received error %v", err)
	}

	if err := copyAssets(templateDir+"/pages", outputDir); err != nil {
		log.Fatalf("Error copying assets. Received error %v", err)
	}

	if err := generatePages(config, templateDir+"/pages", outputDir); err != nil {
		log.Fatalf("Error generating pages. Received error %v", err)
	}

	if err := generateCollections(config, templateDir+"/pages", outputDir); err != nil {
		log.Fatalf("Error generating collections. Received error %v", err)
	}

	log.Println("Portfolio generation completed successfully!")
	log.Printf("Your portfolio is ready in the '%s' directory.", outputDir)
}

func getArgs() (string, string, string) {
	templateId := flag.String("template", "", "Downloads the configuration for a specific template.")
	configFile := flag.String("config", "config.yml", "Specifies the path to the configuration file.")
	outputDir := flag.String("output-dir", ".", "Sets the output directory for the generated site.")

	flag.Parse()

	return *templateId, *configFile, *outputDir
}

func readConfig(configFile string) Config {
	yamlFile, err := os.ReadFile(configFile)
	if err != nil {
		log.Fatalf("Error reading config file: %v. Make sure to run with --template first to download a config file.", err)
	}

	var config Config
	if err := yaml.Unmarshal(yamlFile, &config); err != nil {
		log.Fatalf("Error parsing YAML: %v", err)
	}

	return config
}

func validateConfig(uri string, config Config) {
	schemaLoader := gojsonschema.NewReferenceLoader(uri)
	jsonBytes, err := json.Marshal(config)
	if err != nil {
		log.Fatalf("Error converting config to JSON: %v", err)
	}
	documentLoader := gojsonschema.NewBytesLoader(jsonBytes)

	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		log.Fatalf("Error validating schema: %v", err)
	}

	if result.Valid() {
		log.Println("Config is valid!")
	} else {
		for _, desc := range result.Errors() {
			log.Printf("- %s\n", desc)
		}
		log.Fatalf("Config validation failed, please fix the errors")
	}
}

func generatePages(config Config, templateDir, outputDir string) error {
	for pageName := range config.Pages {
		templatePath := filepath.Join(templateDir, pageName+".html")
		var tmpl *template.Template
		var err error

		if config.Base != nil {
			var basePath = filepath.Join(templateDir, "base.html")
			tmpl, err = template.ParseFiles(basePath, templatePath)
		} else {
			tmpl, err = template.ParseFiles(templatePath)
		}

		if err != nil {
			return fmt.Errorf("error parsing templates for page %s: %v", pageName, err)
		}

		outputFile := filepath.Join(outputDir, pageName+".html")
		f, err := os.Create(outputFile)
		if err != nil {
			return fmt.Errorf("error creating output file %s: %v", outputFile, err)
		}
		defer f.Close()

		if config.Base != nil {
			data := map[string]interface{}{"base": config.Base}
			if pageCtx, ok := config.Pages[pageName].(map[string]interface{}); ok {
				for k, v := range pageCtx {
					data[k] = v
				}
			}
			err = tmpl.ExecuteTemplate(f, "base.html", data)
		} else {
			err = tmpl.Execute(f, config.Pages[pageName])
		}

		if err != nil {
			return fmt.Errorf("error executing template for page %s: %v", pageName, err)
		}

		log.Println("Generated page:", outputFile)
	}

	return nil
}

func generateCollections(config Config, templateDir, outputDir string) error {
	if config.Collections == nil {
		return nil
	}

	for collectionName, collectionData := range config.Collections {
		itemsList, ok := collectionData.(map[string]interface{})["items"].([]interface{})
		if !ok {
			continue
		}

		templatePath := filepath.Join(templateDir, collectionName+".html")
		var tmpl *template.Template
		var err error

		if config.Base != nil {
			var basePath = filepath.Join(templateDir, "base.html")
			tmpl, err = template.ParseFiles(basePath, templatePath)
		} else {
			tmpl, err = template.ParseFiles(templatePath)
		}

		if err != nil {
			return fmt.Errorf("error parsing templates for collection %s: %v", collectionName, err)
		}

		for _, item := range itemsList {
			outputFile, ok := item.(map[string]interface{})["output_file"]
			if !ok {
				continue
			}

			outputFileName, ok := outputFile.(string)
			if !ok {
				continue
			}

			outputPath := filepath.Join(outputDir, outputFileName)
			f, err := os.Create(outputPath)
			if err != nil {
				return fmt.Errorf("error creating output file %s: %v", outputPath, err)
			}
			defer f.Close()

			if config.Base != nil {
				data := map[string]interface{}{"base": config.Base}
				if pageCtx, ok := item.(map[string]interface{}); ok {
					for k, v := range pageCtx {
						data[k] = v
					}
				}
				err = tmpl.ExecuteTemplate(f, "base.html", data)
			} else {
				err = tmpl.Execute(f, item)
			}

			if err != nil {
				return fmt.Errorf("error executing template for item %s: %v", outputFileName, err)
			}

			log.Println("Generated collection item:", outputPath)
		}
	}

	return nil
}

func copyAssets(templateDir, outputDir string) error {
	assetFolders := []string{"assets"}
	for _, folder := range assetFolders {
		srcFolder := filepath.Join(templateDir, folder)
		if _, err := os.Stat(srcFolder); os.IsNotExist(err) {
			continue
		}
		dstFolder := filepath.Join(outputDir, folder)

		err := filepath.WalkDir(srcFolder, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}

			relPath, err := filepath.Rel(srcFolder, path)
			if err != nil {
				return err
			}

			dstPath := filepath.Join(dstFolder, relPath)

			if d.IsDir() {
				return os.MkdirAll(dstPath, 0755)
			}

			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return os.WriteFile(dstPath, data, 0644)
		})

		if err == nil {
			log.Println("Copied folder:", dstFolder)
		}
	}

	return nil
}

func downloadTemplateFromGitHub(templateId string) string {
	tempDir, err := os.MkdirTemp("", "portfolio-template-*")
	if err != nil {
		log.Fatalf("error creating temp directory: %v", err)
	}

	templatePath := filepath.Join(tempDir, "templates", templateId)

	log.Printf("Downloading template '%s'...", templateId)

	treeURL := "https://api.github.com/repos/saksham-ghimire/portfolio-builder/git/trees/main?recursive=1"
	resp, err := http.Get(treeURL)
	if err != nil {
		log.Fatalf("error fetching repository tree: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Fatalf("failed to fetch repository tree: HTTP %d", resp.StatusCode)
	}

	var tree GitHubTree
	if err := json.NewDecoder(resp.Body).Decode(&tree); err != nil {
		log.Fatalf("error parsing tree response: %v", err)
	}

	templatePrefix := fmt.Sprintf("templates/%s/", templateId)
	var templateFiles []string
	for _, item := range tree.Tree {
		if strings.HasPrefix(item.Path, templatePrefix) && item.Type == "blob" {
			templateFiles = append(templateFiles, item.Path)
		}
	}

	if len(templateFiles) == 0 {
		log.Fatalf("no files found for template '%s'", templateId)
	}

	for _, filePath := range templateFiles {
		relPath := strings.TrimPrefix(filePath, templatePrefix)
		localPath := filepath.Join(templatePath, relPath)
		if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
			log.Fatalf("error creating directory: %v", err)
		}

		rawURL := fmt.Sprintf("https://raw.githubusercontent.com/saksham-ghimire/portfolio-builder/main/%s", filePath)
		downloadFile(rawURL, localPath)
		log.Printf("Downloaded: %s", relPath)
	}

	log.Printf("Template downloaded to: %s", templatePath)
	return templatePath
}

func downloadFile(url, filepath string) {
	resp, err := http.Get(url)
	if err != nil {
		log.Fatalf("Failed to download file: %s, error: %v", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Fatalf("Failed to download file: %s, status code: %d", url, resp.StatusCode)
	}

	file, err := os.Create(filepath)
	if err != nil {
		log.Fatalf("Failed to create file: %s, error: %v", filepath, err)
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		log.Fatalf("Failed to copy response body to file: %s, error: %v", filepath, err)
	}
}
