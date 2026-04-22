package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/remiges-tech/plaintest/internal/core"
	"github.com/remiges-tech/plaintest/internal/csv"
	"github.com/remiges-tech/plaintest/internal/newman"
	"github.com/remiges-tech/plaintest/internal/payloadsync"
	"github.com/remiges-tech/plaintest/internal/scriptsync"
	"github.com/remiges-tech/plaintest/internal/templates"
)

var rootCmd = &cobra.Command{
	Use:   "plaintest",
	Short: "PlainTest CLI for API testing",
	Long:  "PlainTest provides a framework for API testing using Postman collections and CSV data.",
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the tool version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(core.Version)
	},
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create PlainTest project structure",
	Long:  "Creates the basic PlainTest project structure with collections/, data/, environments/, and reports/ directories plus working template files.",
	Run: func(cmd *cobra.Command, args []string) {
		initializer := templates.NewProjectInitializer()
		if err := initializer.CreateProjectStructure(); err != nil {
			fmt.Printf("Error creating project structure: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("PlainTest project initialized successfully!")
		fmt.Println("Try: plaintest run smoke")
		fmt.Println("Or:  plaintest run --setup get_auth --test api_tests")
		fmt.Println("Or:  plaintest run api_tests -d data/example.csv")
	},
}

var rowSelection string
var debugNewman bool
var generateReports bool
var setupLinks []string
var testLinks []string
var generatedReports []string

// LinkSpec represents a parsed link specification
type LinkSpec struct {
	Collection string
	Items      []string // Empty means whole collection
}

// ExecutionPhase represents setup or test phase
type ExecutionPhase struct {
	Links []LinkSpec
	Phase string // "setup" or "test"
}

// Flag constants for command line flags
const (
	envShortFlag  = "-e"
	envLongFlag   = "--environment"
	dataShortFlag = "-d"
	dataLongFlag  = "--iteration-data"
	rowsShortFlag = "-r"
	rowsLongFlag  = "--rows"
	debugFlag     = "--debug"
	reportsFlag   = "--reports"

	// Newman flag constants
	reportersFlag    = "--reporters"
	jsonReporter     = "json"
	defaultReporters = "cli,htmlextra,json"
	jsonExportFlag   = "--reporter-json-export"
	htmlExportFlag   = "--reporter-htmlextra-export"

	// File constants
	reportsDir      = "reports"
	timestampFormat = "20060102T150405"
)

// parseLinkSpec parses a link specification like "collection.item1,item2"
func parseLinkSpec(linkSpec string) (LinkSpec, error) {
	// Handle quoted strings and dots
	parts := strings.SplitN(linkSpec, ".", 2)
	if len(parts) == 1 {
		// Just collection name
		collection := strings.Trim(parts[0], "\"")
		return LinkSpec{Collection: collection, Items: []string{}}, nil
	}

	// Collection and items
	collection := strings.Trim(parts[0], "\"")
	itemsStr := strings.Trim(parts[1], "\"")

	var items []string
	if itemsStr != "" {
		// Split by comma and trim spaces
		rawItems := strings.Split(itemsStr, ",")
		for _, item := range rawItems {
			trimmed := strings.TrimSpace(item)
			if trimmed != "" {
				items = append(items, trimmed)
			}
		}
	}

	// Ensure items is never nil for consistent comparison
	if items == nil {
		items = []string{}
	}

	return LinkSpec{Collection: collection, Items: items}, nil
}

// parsePhases parses setup and test links into execution phases
func parsePhases(setupLinks, testLinks []string) ([]ExecutionPhase, error) {
	phases := make([]ExecutionPhase, 0)

	// Parse setup phase if any setup links
	if len(setupLinks) > 0 {
		var setupSpecs []LinkSpec
		for _, link := range setupLinks {
			spec, err := parseLinkSpec(link)
			if err != nil {
				return nil, fmt.Errorf("invalid setup link '%s': %v", link, err)
			}
			setupSpecs = append(setupSpecs, spec)
		}
		phases = append(phases, ExecutionPhase{Links: setupSpecs, Phase: "setup"})
	}

	// Parse test phase if any test links
	if len(testLinks) > 0 {
		var testSpecs []LinkSpec
		for _, link := range testLinks {
			spec, err := parseLinkSpec(link)
			if err != nil {
				return nil, fmt.Errorf("invalid test link '%s': %v", link, err)
			}
			testSpecs = append(testSpecs, spec)
		}
		phases = append(phases, ExecutionPhase{Links: testSpecs, Phase: "test"})
	}

	return phases, nil
}

// executeLinkSpec executes a single link specification
func executeLinkSpec(linkSpec LinkSpec, phase string, linkIndex, totalLinks int, config *DiscoveryConfig,
	newmanFlags []string, service *newman.Service, tempEnvFile *string) error {

	// Find collection path
	collectionPath, err := getCollectionPath(linkSpec.Collection, config)
	if err != nil {
		return err
	}

	// Build flags for this link
	currentFlags := make([]string, len(newmanFlags))
	copy(currentFlags, newmanFlags)

	// Add folder selections if any
	for _, item := range linkSpec.Items {
		currentFlags = append(currentFlags, "--folder", item)
	}

	// For setup phase, remove CSV iteration flags
	if phase == "setup" {
		currentFlags = removeCsvFlags(currentFlags)
	}

	// Apply row selection if specified and this is test phase
	if phase == "test" && rowSelection != "" {
		currentFlags = applyRowSelection(currentFlags, rowSelection)
	}

	// Use shared environment from previous link
	if *tempEnvFile != "" {
		currentFlags = replaceEnvironmentInFlags(currentFlags, *tempEnvFile)
	}

	// Add report flags if requested
	if generateReports {
		currentFlags = addReportFlags(currentFlags, linkSpec.Collection)
	}

	// Print execution status
	printLinkStatus(linkSpec, phase, linkIndex, totalLinks)

	var result *newman.Result

	// If not the last link, export environment for next link
	if linkIndex < totalLinks {
		if *tempEnvFile == "" {
			*tempEnvFile, err = createTempEnvironmentFile()
			if err != nil {
				return fmt.Errorf("creating temporary environment file: %v", err)
			}
		}
		result, err = service.RunWithEnvironmentExport(collectionPath, currentFlags, *tempEnvFile)
	} else {
		result, err = service.RunWithFlags(collectionPath, currentFlags)
	}

	if err != nil {
		if result != nil && result.Output != "" {
			fmt.Println("Newman output:")
			fmt.Println(result.Output)
		}
		return fmt.Errorf("execution failed: %v", err)
	}

	return handleResult(result, linkSpec.Collection, currentFlags)
}

// getCollectionPath validates and returns the collection path
func getCollectionPath(collectionName string, config *DiscoveryConfig) (string, error) {
	collectionPath, exists := config.Collections[collectionName]
	if !exists {
		availableCollections := make([]string, 0, len(config.Collections))
		for name := range config.Collections {
			availableCollections = append(availableCollections, name)
		}
		return "", fmt.Errorf("unknown collection: %s. Available: %v", collectionName, availableCollections)
	}
	return collectionPath, nil
}

// handleResult processes Newman execution result and returns appropriate error
func handleResult(result *newman.Result, collectionName string, flags []string) error {
	if result.Success {
		if isVerboseMode(flags) && result.Output != "" {
			fmt.Println(result.Output)
		} else {
			fmt.Printf("%s link: All tests passed!\n", collectionName)
		}
		return nil
	}

	fmt.Printf("%s link: Tests completed with exit code: %d\n", collectionName, result.ExitCode)
	if result.Output != "" {
		fmt.Println("Newman output:")
		fmt.Println(result.Output)
	}
	return fmt.Errorf("tests failed with exit code %d", result.ExitCode)
}

// removeCsvFlags removes CSV iteration flags for setup phase
func removeCsvFlags(flags []string) []string {
	result := make([]string, 0, len(flags))
	for i := 0; i < len(flags); i++ {
		flag := flags[i]
		if flag == "-d" || flag == "--iteration-data" {
			// Skip this flag and its value (if exists and doesn't start with -)
			if i+1 < len(flags) && !strings.HasPrefix(flags[i+1], "-") {
				i++ // Skip the value too
			}
		} else {
			result = append(result, flag)
		}
	}
	return result
}

// printLinkStatus prints the status of the current link being executed
func printLinkStatus(linkSpec LinkSpec, phase string, linkIndex, totalLinks int) {
	prefix := fmt.Sprintf("Running %s link %d/%d", phase, linkIndex, totalLinks)
	if len(linkSpec.Items) > 0 {
		fmt.Printf("%s: %s.%s\n", prefix, linkSpec.Collection, strings.Join(linkSpec.Items, ","))
	} else {
		fmt.Printf("%s: %s\n", prefix, linkSpec.Collection)
	}
}

// buildRunCommandLong creates the long description with available collections
func buildRunCommandLong() string {
	config := discoverAllFiles()

	availableNames := make([]string, 0, len(config.Collections))
	for name := range config.Collections {
		availableNames = append(availableNames, name)
	}

	envNames := make([]string, 0, len(config.Environments))
	for name := range config.Environments {
		envNames = append(envNames, name)
	}

	dataNames := make([]string, 0, len(config.DataFiles))
	for name := range config.DataFiles {
		dataNames = append(dataNames, name)
	}

	return fmt.Sprintf(`Execute API tests using Newman with setup and test phases.

Available collections: %v
Available environments: %v
Available data files: %v

Examples:
  plaintest run --test smoke
  plaintest run --setup auth --test api_tests -d data/example.csv
  plaintest run --setup "auth.Login" --test "api tests.User Tests" -d data.csv
  plaintest run --test "api tests.Create User,Update User,Delete User"
  plaintest run --setup db.Init --setup auth.Login --test api_tests --reports

Setup phase runs once, test phase iterates with CSV data.
All Newman flags are supported. PlainTest-specific flags are listed below.`, availableNames, envNames, dataNames)
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Execute API tests with Newman",
	Long:  buildRunCommandLong(),
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		// Clear any previously tracked reports
		generatedReports = nil

		// Validate that at least setup or test is specified
		if len(setupLinks) == 0 && len(testLinks) == 0 {
			fmt.Println("Error: Must specify at least one --setup or --test link")
			fmt.Println("Examples:")
			fmt.Println("  plaintest run --test smoke")
			fmt.Println("  plaintest run --setup auth --test api_tests")
			os.Exit(1)
		}

		service := newman.NewService()
		service.SetDebug(debugNewman)

		if !service.IsInstalled() {
			fmt.Println("Error: Newman is not installed. Install with: npm install -g newman newman-reporter-htmlextra")
			os.Exit(1)
		}

		// Discover available collections, environments, and data files
		config := discoverAllFiles()

		// Parse setup and test phases
		phases, err := parsePhases(setupLinks, testLinks)
		if err != nil {
			fmt.Printf("Error parsing links: %v\n", err)
			os.Exit(1)
		}

		// Get Newman flags from raw args (skip "plaintest run")
		rawArgs := os.Args[2:] // Skip "plaintest run"
		_, newmanFlags, err := parseArguments(rawArgs, config)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		// Add default environment if not specified and only one environment exists
		if !hasEnvironmentFlag(newmanFlags) {
			if len(config.Environments) == 1 {
				for _, envPath := range config.Environments {
					newmanFlags = append(newmanFlags, "-e", envPath)
					break
				}
			}
		}

		// Execute phases in order: setup first, then test
		var tempEnvFile string
		var exitCode int
		defer func() {
			if tempEnvFile != "" {
				cleanupTempFile(tempEnvFile)
			}
		}()

		var linkIndex int
		for _, phase := range phases {
			for _, linkSpec := range phase.Links {
				linkIndex++
				err := executeLinkSpec(linkSpec, phase.Phase, linkIndex, len(setupLinks)+len(testLinks),
					&config, newmanFlags, service, &tempEnvFile)
				if err != nil {
					fmt.Printf("Error executing %s link '%s': %v\n", phase.Phase, linkSpec.Collection, err)
					exitCode = 1
					break
				}
			}
			if exitCode != 0 {
				break
			}
		}

		// Show summary of generated reports
		if len(generatedReports) > 0 {
			fmt.Println()
			fmt.Println("Generated Reports:")
			for _, report := range generatedReports {
				if strings.HasSuffix(report, ".json") {
					fmt.Printf("   JSON: %s\n", report)
				} else {
					fmt.Printf("   HTML: %s\n", report)
				}
			}
		}

		if exitCode != 0 {
			os.Exit(exitCode)
		}
	},
}

var scriptsCmd = &cobra.Command{
	Use:   "scripts",
	Short: "Manage collection scripts",
	Long:  "Pull scripts from collections to editable files or push edited scripts back to collections.",
}

var scriptsPullCmd = &cobra.Command{
	Use:   "pull [collection-name]",
	Short: "Pull scripts from collection to editable JS files",
	Long:  "Pulls all scripts from a Postman collection to individual JavaScript files for editing outside Postman.",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		collectionName := args[0]
		service := scriptsync.NewService(scriptsync.Config{})
		if err := service.Extract(collectionName); err != nil {
			fmt.Printf("Error pulling scripts: %v\n", err)
			os.Exit(1)
		}
	},
}

var scriptsPushCmd = &cobra.Command{
	Use:   "push [collection-name]",
	Short: "Push updated scripts from JS files to collection",
	Long:  "Pushes scripts from edited JavaScript files back to the Postman collection. Scripts are the source of truth.",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		collectionName := args[0]
		service := scriptsync.NewService(scriptsync.Config{})
		if err := service.Build(collectionName); err != nil {
			fmt.Printf("Error pushing scripts: %v\n", err)
			os.Exit(1)
		}
	},
}

var payloadsCmd = &cobra.Command{
	Use:   "payloads",
	Short: "Manage request payloads",
	Long:  "Pull request bodies from collections to editable JSON files or push edited payloads back to collections.",
}

var payloadsPullCmd = &cobra.Command{
	Use:   "pull [collection-name]",
	Short: "Pull request bodies from collection to JSON files",
	Long:  "Pulls all request bodies from a Postman collection to individual JSON files for editing.",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		collectionName := args[0]
		service := payloadsync.NewService(payloadsync.Config{})
		if err := service.Extract(collectionName); err != nil {
			fmt.Printf("Error pulling payloads: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Successfully extracted payloads from %s to payloads/%s/\n", collectionName, collectionName)
	},
}

var payloadsPushCmd = &cobra.Command{
	Use:   "push [collection-name]",
	Short: "Push updated payloads from JSON files to collection",
	Long:  "Pushes request bodies from edited JSON files back to the Postman collection. Payload files are the source of truth.",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		collectionName := args[0]
		service := payloadsync.NewService(payloadsync.Config{})
		if err := service.Build(collectionName); err != nil {
			fmt.Printf("Error pushing payloads: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Successfully updated %s with payloads from payloads/%s/\n", collectionName, collectionName)
	},
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List project resources",
	Long:  "List available collections, data files, scripts, or environments in the current PlainTest project.",
}

var listCollectionsCmd = &cobra.Command{
	Use:   "collections",
	Short: "List available collections",
	Long:  "Lists all Postman collections found in the collections directory.",
	Run: func(cmd *cobra.Command, args []string) {
		config := discoverAllFiles()
		if len(config.Collections) == 0 {
			fmt.Println("No collections found in collections/ directory")
			return
		}

		fmt.Println("Available collections:")
		for name, path := range config.Collections {
			fmt.Printf("  %s (%s)\n", name, path)
		}
	},
}

var listDataCmd = &cobra.Command{
	Use:   "data",
	Short: "List available data files",
	Long:  "Lists all CSV data files found in the data directory.",
	Run: func(cmd *cobra.Command, args []string) {
		config := discoverAllFiles()
		if len(config.DataFiles) == 0 {
			fmt.Println("No data files found in data/ directory")
			return
		}

		fmt.Println("Available data files:")
		for name, path := range config.DataFiles {
			fmt.Printf("  %s (%s)\n", name, path)
		}
	},
}

var listEnvironmentsCmd = &cobra.Command{
	Use:   "environments",
	Short: "List available environments",
	Long:  "Lists all Postman environment files found in the environments directory.",
	Run: func(cmd *cobra.Command, args []string) {
		config := discoverAllFiles()
		if len(config.Environments) == 0 {
			fmt.Println("No environments found in environments/ directory")
			return
		}

		fmt.Println("Available environments:")
		for name, path := range config.Environments {
			fmt.Printf("  %s (%s)\n", name, path)
		}
	},
}

var listScriptsCmd = &cobra.Command{
	Use:   "scripts",
	Short: "List extracted scripts",
	Long:  "Lists all extracted script directories found in the scripts directory.",
	Run: func(cmd *cobra.Command, args []string) {
		scriptsDir := "scripts"

		entries, err := os.ReadDir(scriptsDir)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("No scripts directory found")
				return
			}
			fmt.Printf("Error reading scripts directory: %v\n", err)
			return
		}

		scriptDirs := make([]string, 0)
		for _, entry := range entries {
			if entry.IsDir() {
				scriptDirs = append(scriptDirs, entry.Name())
			}
		}

		if len(scriptDirs) == 0 {
			fmt.Println("No extracted scripts found in scripts/ directory")
			fmt.Println("Use 'plaintest scripts pull [collection]' to pull scripts")
			return
		}

		fmt.Println("Available script directories:")
		for _, dir := range scriptDirs {
			scriptPath := filepath.Join(scriptsDir, dir)
			fileCount := countScriptFiles(scriptPath)
			fmt.Printf("  %s (%d script files)\n", dir, fileCount)
		}
	},
}

// countScriptFiles counts the number of .js files in a directory recursively
func countScriptFiles(dir string) int {
	count := 0
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors and continue
		}
		if !info.IsDir() && strings.HasSuffix(path, ".js") {
			count++
		}
		return nil
	})
	return count
}

// DiscoveryConfig holds all discovered files
type DiscoveryConfig struct {
	Collections  map[string]string
	Environments map[string]string
	DataFiles    map[string]string
}

// discoverFiles scans directories for files matching patterns and suffix
func discoverFiles(patterns []string, suffix string) map[string]string {
	fileMap := make(map[string]string)

	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			fmt.Printf("Warning: Could not scan directory for %s: %v\n", pattern, err)
			continue
		}
		if len(matches) == 0 {
			continue
		}

		for _, filePath := range matches {
			filename := filepath.Base(filePath)
			name := strings.TrimSuffix(filename, suffix)
			fileMap[name] = filePath
		}

		// Stop after first pattern with matches (build/ has priority over raw collections/)
		if len(fileMap) > 0 {
			break
		}
	}

	return fileMap
}

// discoverAllFiles discovers all collections, environments, and data files
func discoverAllFiles() DiscoveryConfig {
	return DiscoveryConfig{
		Collections: discoverFiles([]string{
			filepath.Join("collections", "build", "*.postman_collection.json"),
			filepath.Join("collections", "*.postman_collection.json"),
		}, ".postman_collection.json"),
		Environments: discoverFiles([]string{
			filepath.Join("environments", "*.postman_environment.json"),
		}, ".postman_environment.json"),
		DataFiles: discoverFiles([]string{
			filepath.Join("data", "*.csv"),
		}, ".csv"),
	}
}

// resolveFilePathFromName attempts to resolve a name to a file path using the lookup map
func resolveFilePathFromName(name string, lookupMap map[string]string) string {
	if resolvedPath, exists := lookupMap[name]; exists {
		return resolvedPath
	}
	// Use as-is (might already be a full path)
	return name
}

// parseArguments separates collection names from Newman flags
func parseArguments(args []string, config DiscoveryConfig) (collections []string, newmanFlags []string, err error) {
	// Map of flags that need path resolution
	flagMaps := map[string]map[string]string{
		envShortFlag:  config.Environments,
		envLongFlag:   config.Environments,
		dataShortFlag: config.DataFiles,
		dataLongFlag:  config.DataFiles,
	}

	argIndex := 0
	for argIndex < len(args) {
		arg := args[argIndex]

		// Skip PlainTest-specific flags
		if skip := skipPlainTestFlag(arg, &argIndex); skip {
			continue
		}

		if _, exists := config.Collections[arg]; exists {
			collections = append(collections, arg)
			argIndex++
		} else if strings.HasPrefix(arg, "-") {
			// Process Newman flag
			argIndex = processNewmanFlag(arg, args, argIndex, flagMaps, &newmanFlags)
		} else {
			// Non-flag argument that's not a known collection - treat as invalid collection
			return collections, newmanFlags, fmt.Errorf("unknown collection: %s. Available: %v", arg, getAvailableCollections(config))
		}
	}
	return collections, newmanFlags, nil
}

// skipPlainTestFlag checks if argument is a PlainTest-specific flag and advances index
func skipPlainTestFlag(arg string, argIndex *int) bool {
	if arg == rowsShortFlag || arg == rowsLongFlag {
		*argIndex += 2 // Skip flag and its value
		return true
	}
	if arg == "--setup" || arg == "--test" {
		*argIndex += 2 // Skip flag and its value
		return true
	}
	if arg == debugFlag || arg == reportsFlag {
		*argIndex++
		return true
	}
	return false
}

// processNewmanFlag processes a Newman flag and returns new index
func processNewmanFlag(arg string, args []string, argIndex int, flagMaps map[string]map[string]string, newmanFlags *[]string) int {
	*newmanFlags = append(*newmanFlags, arg)
	argIndex++

	// Check if this flag needs path resolution
	if lookupMap, needsResolution := flagMaps[arg]; needsResolution {
		if argIndex < len(args) && !strings.HasPrefix(args[argIndex], "-") {
			value := args[argIndex]
			resolvedPath := resolveFilePathFromName(value, lookupMap)
			*newmanFlags = append(*newmanFlags, resolvedPath)
			argIndex++
		}
	} else {
		// Check if this flag expects a value (next arg doesn't start with -)
		if argIndex < len(args) && !strings.HasPrefix(args[argIndex], "-") {
			// Add the flag value
			*newmanFlags = append(*newmanFlags, args[argIndex])
			argIndex++
		}
	}
	return argIndex
}

// getAvailableCollections returns a slice of available collection names
func getAvailableCollections(config DiscoveryConfig) []string {
	availableNames := make([]string, 0, len(config.Collections))
	for name := range config.Collections {
		availableNames = append(availableNames, name)
	}
	return availableNames
}

// extractCSVFromFlags finds CSV file specified in Newman flags
func extractCSVFromFlags(flags []string) string {
	for i, flag := range flags {
		if flag == "-d" || flag == "--iteration-data" {
			if i+1 < len(flags) {
				return flags[i+1]
			}
		}
	}
	return ""
}

// replaceCSVInFlags replaces CSV file in Newman flags with new file
func replaceCSVInFlags(flags []string, newCSVFile string) []string {
	result := make([]string, len(flags))
	copy(result, flags)

	for i, flag := range result {
		if flag == "-d" || flag == "--iteration-data" {
			if i+1 < len(result) {
				result[i+1] = newCSVFile
			}
		}
	}
	return result
}

// hasEnvironmentFlag checks if environment is already specified in flags
func hasEnvironmentFlag(flags []string) bool {
	for _, flag := range flags {
		if flag == "-e" || flag == "--environment" {
			return true
		}
	}
	return false
}

func applyRowSelection(flags []string, rowSelection string) []string {
	csvFile := extractCSVFromFlags(flags)
	if csvFile == "" {
		fmt.Println("Warning: Row selection specified but no CSV file found in flags")
		return flags
	}

	processor := csv.NewProcessor()
	tempCSVFile, err := processor.ProcessRows(csvFile, rowSelection)
	if err != nil {
		fmt.Printf("Error processing CSV rows: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Using row selection: %s from %s\n", rowSelection, csvFile)
	return replaceCSVInFlags(flags, tempCSVFile)
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(scriptsCmd)
	rootCmd.AddCommand(payloadsCmd)
	rootCmd.AddCommand(listCmd)

	scriptsCmd.AddCommand(scriptsPullCmd)
	scriptsCmd.AddCommand(scriptsPushCmd)

	payloadsCmd.AddCommand(payloadsPullCmd)
	payloadsCmd.AddCommand(payloadsPushCmd)

	listCmd.AddCommand(listCollectionsCmd)
	listCmd.AddCommand(listDataCmd)
	listCmd.AddCommand(listEnvironmentsCmd)
	listCmd.AddCommand(listScriptsCmd)

	// Only PlainTest-specific flags
	runCmd.Flags().StringSliceVar(&setupLinks, "setup", []string{}, "Setup links to run once (collection or collection.items)")
	runCmd.Flags().StringSliceVar(&testLinks, "test", []string{}, "Test links to run with CSV iteration (collection or collection.items)")
	runCmd.Flags().StringVarP(&rowSelection, "rows", "r", "", "CSV row selection (2 | 2-5 | 1,3,5)")
	runCmd.Flags().BoolVar(&debugNewman, "debug", false, "Print the Newman command before running")
	runCmd.Flags().BoolVar(&generateReports, "reports", false, "Generate timestamped HTML and JSON report files")

	// Allow unknown flags to be passed to Newman
	runCmd.FParseErrWhitelist.UnknownFlags = true
}

// createTempEnvironmentFile creates a temporary environment file for collection chaining
func createTempEnvironmentFile() (string, error) {
	tmpDir := os.TempDir()
	timestamp := time.Now().UnixNano()
	tempFile := filepath.Join(tmpDir, fmt.Sprintf("plaintest_env_%d.json", timestamp))
	return tempFile, nil
}

// replaceEnvironmentInFlags replaces environment file in Newman flags
func replaceEnvironmentInFlags(flags []string, newEnvFile string) []string {
	result := make([]string, 0, len(flags))

	for i := 0; i < len(flags); i++ {
		flag := flags[i]
		if flag == "-e" || flag == "--environment" {
			// Replace environment flag and its value
			result = append(result, flag)
			if i+1 < len(flags) {
				result = append(result, newEnvFile)
				i++ // Skip the old environment file
			}
		} else {
			result = append(result, flag)
		}
	}

	return result
}

// cleanupTempFile removes temporary environment file
func cleanupTempFile(filePath string) {
	if filePath != "" {
		os.Remove(filePath)
	}
}

func addReportFlags(flags []string, collectionName string) []string {
	flags = ensureJSONReporter(flags)
	flags = addExportPaths(flags, collectionName)
	return flags
}

func ensureJSONReporter(flags []string) []string {
	for i, flag := range flags {
		if flag == reportersFlag {
			if needsJSONReporter(flags, i) {
				flags[i+1] = flags[i+1] + "," + jsonReporter
			}
			return flags
		}
	}
	return append(flags, reportersFlag, defaultReporters)
}

func needsJSONReporter(flags []string, reportersIndex int) bool {
	reportersValueIndex := reportersIndex + 1
	return reportersValueIndex < len(flags) && !strings.Contains(flags[reportersValueIndex], jsonReporter)
}

func addExportPaths(flags []string, collectionName string) []string {
	timestamp := time.Now().Format(timestampFormat)
	jsonFile := filepath.Join(reportsDir, fmt.Sprintf("%s_%s.json", collectionName, timestamp))
	htmlFile := filepath.Join(reportsDir, fmt.Sprintf("%s_%s.html", collectionName, timestamp))

	flags = append(flags, jsonExportFlag, jsonFile)
	flags = append(flags, htmlExportFlag, htmlFile)

	// Track generated reports for summary at the end
	generatedReports = append(generatedReports, jsonFile, htmlFile)

	return flags
}

func isVerboseMode(flags []string) bool {
	for _, flag := range flags {
		if flag == "--verbose" {
			return true
		}
	}
	return false
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
