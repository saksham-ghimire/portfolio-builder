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
	// Parse command line arguments
	templateId, configFilePath, outputDir := getArgs()
	configUrl := fmt.Sprintf("https://raw.githubusercontent.com/saksham-ghimire/portfolio-builder/main/templates/%s/config.yml", templateId)

	if templateId != "" {
		log.Println("Fetching template configuration for id:", templateId)
		downloadFile(configUrl, "config.yml")
		log.Println("Successfully fetched the configuration, please update 'config.yml' as needed, and execute the program")
		return
	}

	var config = readConfig(configFilePath)
	schemaUrl := fmt.Sprintf("https://raw.githubusercontent.com/saksham-ghimire/portfolio-builder/main/templates/%s/schema.json", config.TemplateId)
	validateConfig(schemaUrl, config)

	// Download template from GitHub
	templateDir := downloadTemplateFromGitHub(config.TemplateId)
	defer os.RemoveAll(templateDir)

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatalf("Error creating output directory. Received error %v", err)
	}

	// Copy assets
	if err := copyAssets(templateDir+"/pages", outputDir); err != nil {
		log.Fatalf("Error copying assets. Received error %v", err)
	}

	// Generate pages based on config.pages
	if err := generatePages(config, templateDir+"/pages", outputDir); err != nil {
		log.Fatalf("Error generating pages. Received error %v", err)
	}

	// Generate collection pages
	if err := generateCollections(config, templateDir+"/pages", outputDir); err != nil {
		log.Fatalf("Error generating collections. Received error %v", err)
	}

	log.Println("Portfolio generation completed successfully!")
}

func getArgs() (string, string, string) {

	templateId := flag.String("template", "", "Configuration to use")

	// both of these default should be fine
	configFile := flag.String("config", "config.yml", "Configuration file path")
	outputDir := flag.String("output-dir", ".", "Output directory path")

	flag.Parse()

	return *templateId, *configFile, *outputDir
}

func readConfig(configFile string) Config {
	// Load YAML data
	yamlFile, err := os.ReadFile(configFile)
	if err != nil {
		log.Fatalf("Error reading config file: %v", err)
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

	// Validate
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
		log.Fatalf("Config validation failed, please fix the resulted error and try again")
	}

}

func generatePages(config Config, templateDir, outputDir string) error {

	for pageName := range config.Pages {

		templatePath := filepath.Join(templateDir, pageName+".html")

		var tmpl *template.Template
		var err error

		// Parse templates based on whether base template exists
		if config.Base != nil {
			var basePath = filepath.Join(templateDir, "base.html")
			tmpl, err = template.ParseFiles(basePath, templatePath)
		} else {
			tmpl, err = template.ParseFiles(templatePath)
		}

		if err != nil {
			return fmt.Errorf("error parsing templates for page %s: %v", pageName, err)
		}

		// Create output file
		outputFile := filepath.Join(outputDir, pageName+".html")
		f, err := os.Create(outputFile)
		if err != nil {
			return fmt.Errorf("error creating output file %s: %v", outputFile, err)
		}
		defer f.Close()

		// Execute template
		if config.Base != nil {
			data := map[string]interface{}{
				"base": config.Base,
			}
			// merge the page-specific context
			if pageCtx, ok := config.Pages[pageName].(map[string]interface{}); ok {
				for k, v := range pageCtx {
					data[k] = v
				}
			}
			err = tmpl.ExecuteTemplate(f, "base.html", data)
		} else {
			// why is it when base is nil the generate output file is always empty
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

		// Use collection name as template name
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

		// Generate a page for each item in the collection
		for _, item := range itemsList {
			outputFile, ok := item.(map[string]interface{})["output_file"]
			if !ok {
				continue
			}

			outputFileName, ok := outputFile.(string)
			if !ok {
				continue
			}

			// Create output file
			outputPath := filepath.Join(outputDir, outputFileName)
			f, err := os.Create(outputPath)
			if err != nil {
				return fmt.Errorf("error creating output file %s: %v", outputPath, err)
			}
			defer f.Close()

			// Execute template
			if config.Base != nil {
				data := map[string]interface{}{
					"base": config.Base,
				}
				// merge the page-specific context
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

	// Copy asset folders
	assetFolders := []string{"assets"}
	for _, folder := range assetFolders {
		srcFolder := filepath.Join(templateDir, folder)
		if _, err := os.Stat(srcFolder); os.IsNotExist(err) {
			continue
		}
		dstFolder := filepath.Join(outputDir, folder)

		// Copy folder recursively
		err := filepath.WalkDir(srcFolder, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}

			// Get relative path from source folder
			relPath, err := filepath.Rel(srcFolder, path)
			if err != nil {
				return err
			}

			dstPath := filepath.Join(dstFolder, relPath)

			if d.IsDir() {
				return os.MkdirAll(dstPath, 0755)
			}

			// Copy file
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
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "portfolio-template-*")
	if err != nil {
		log.Fatalf("error creating temp directory: %v", err)
	}

	templatePath := filepath.Join(tempDir, "templates", templateId)

	log.Printf("Downloading template '%s'...", templateId)

	// Get repository tree
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

	// Filter files for the specific template
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

	// Download each file
	for _, filePath := range templateFiles {
		relPath := strings.TrimPrefix(filePath, templatePrefix)
		localPath := filepath.Join(templatePath, relPath)

		// Create directory if needed
		if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
			log.Fatalf("error creating directory: %v", err)
		}

		// Download file
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
