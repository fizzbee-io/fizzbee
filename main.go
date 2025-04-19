package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	ast "fizz/proto"
	"flag"
	"fmt"
	"github.com/fizzbee-io/fizzbee/lib"
	"github.com/fizzbee-io/fizzbee/modelchecker"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"html"
	"html/template"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/pprof"
	"slices"
	"strings"
	"syscall"
	"time"
)

var isPlayground bool
var simulation bool
var internalProfile bool
var saveStates bool
var seed int64
var maxRuns int
var explorationStrategy string

func main() {
	flag.BoolVar(&isPlayground, "playground", false, "is for playground")
	flag.BoolVar(&simulation, "simulation", false, "Runs in simulation mode (DFS). Default=false for no simulation (BFS)")
	flag.BoolVar(&internalProfile, "internal_profile", false, "Enables CPU and memory profiling of the model checker")
	flag.BoolVar(&saveStates, "save_states", false, "Save states to disk")
	flag.Int64Var(&seed, "seed", 0, "Seed for random number generator used in simulation mode")
	flag.IntVar(&maxRuns, "max_runs", 0, "Maximum number of simulation runs/paths to explore. Default=0 for unlimited")
        flag.StringVar(&explorationStrategy, "exploration_strategy", "bfs", "Exploration strategy for exhaustive model checking. Options: bfs (default), dfs, random.")
	flag.Parse()

	args := flag.Args()
	// Check if the correct number of arguments is provided
	if len(args) != 1 {
		fmt.Println("Usage:", os.Args[0], "<json_file>")
		os.Exit(1)
	}

	// Get the input JSON file name from command line argument
	jsonFilename := args[0]

	// Read the content of the JSON file
	jsonContent, err := os.ReadFile(jsonFilename)
	if err != nil {
		fmt.Println("Error reading JSON file:", err)
		os.Exit(1)
	}
	f := &ast.File{}
	err = protojson.Unmarshal(jsonContent, f)
	if err != nil {
		fmt.Println("Error unmarshalling JSON:", err)
		os.Exit(1)
	}

	dirPath := filepath.Dir(jsonFilename)

	sourceFileName := filepath.Join(dirPath, f.SourceInfo.GetFileName())
	//fmt.Println("dirPath:", dirPath)
	// Calculate the relative path
	configFileName := filepath.Join(dirPath, "fizz.yaml")
	fmt.Println("configFileName:", configFileName)
	stateConfig, err := modelchecker.ReadOptionsFromYaml(configFileName)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if isPlayground {
				deadlockDetection := true
				crashOnYield := true
				stateConfig = &ast.StateSpaceOptions{
					Options:           &ast.Options{MaxActions: 100, MaxConcurrentActions: 2, CrashOnYield: &crashOnYield},
					Liveness:          "strict",
					DeadlockDetection: &deadlockDetection,
				}
			} else {
				fmt.Println("fizz.yaml not found. Using default options")
				stateConfig = &ast.StateSpaceOptions{Options: &ast.Options{MaxActions: 100, MaxConcurrentActions: 2}}
			}
		} else {
			fmt.Println("Error reading fizz.yaml:", err)
			os.Exit(1)
		}

	}
	if f.GetFrontMatter().GetYaml() != "" {
		fmStateConfig, err := modelchecker.ReadOptionsFromYamlString(f.GetFrontMatter().GetYaml())
		if err != nil {
			fmt.Println("Error parsing YAML frontmatter:", err)
			os.Exit(1)
		}
		proto.Merge(stateConfig, fmStateConfig)
	}

	fmt.Printf("StateSpaceOptions: %+v\n", stateConfig)
	if stateConfig.Options.MaxActions == 0 {
		stateConfig.Options.MaxActions = 100
	}
	if stateConfig.Options.MaxConcurrentActions == 0 {
		stateConfig.Options.MaxConcurrentActions = min(2, stateConfig.Options.MaxActions)
	}
	if stateConfig.DeadlockDetection == nil {
		deadlockDetection := true
		stateConfig.DeadlockDetection = &deadlockDetection
	}
	if stateConfig.Options.CrashOnYield == nil {
		crashOnYield := true
		stateConfig.Options.CrashOnYield = &crashOnYield
	}
	outDir, err := createOutputDir(dirPath)
	if err != nil {
		return
	}

	//maxRuns := 10000
	if !simulation || seed != 0 {
		maxRuns = 1
	}
	if simulation && seed != 0 {
		fmt.Println("Seed:", seed)
	}
	if simulation && maxRuns == 0 {
		fmt.Println("MaxRuns: unlimited")
	}
	stopped := false
	runs := 0
	var p1 *modelchecker.Processor
	if simulation {
		c := make(chan os.Signal)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-c
			fmt.Println("\nInterrupted. Stopping state exploration")
			stopped = true
			if p1 != nil {
				p1.Stop()
			}
		}()
	}
	i := 0
	for !stopped && (maxRuns <= 0 || i < maxRuns) {
		i++

		p1 = modelchecker.NewProcessor([]*ast.File{f}, stateConfig, simulation, seed, dirPath, explorationStrategy)
		if !simulation {
			c := make(chan os.Signal)
			signal.Notify(c, os.Interrupt, syscall.SIGTERM)
			go func() {
				<-c
				fmt.Println("\nInterrupted. Stopping state exploration")
				p1.Stop()
			}()
		}

		rootNode, failedNode, endTime := startModelChecker(err, p1)
		runs++

		if p1.GetVisitedNodesCount() < 250 {
			dotString := modelchecker.GenerateDotFile(rootNode, make(map[*modelchecker.Node]bool))
			dotFileName := filepath.Join(outDir, "graph.dot")
			// Write the content to the file
			err := os.WriteFile(dotFileName, []byte(dotString), 0644)
			if err != nil {
				fmt.Println("Error writing to file:", err)
				return
			}
			if !isPlayground && !simulation {
				fmt.Printf("Writen graph dotfile: %s\nTo generate svg, run: \n"+
					"dot -Tsvg %s -o graph.svg && open graph.svg\n", dotFileName, dotFileName)
			}
		} else if !simulation {
			fmt.Printf("Skipping dotfile generation. Too many nodes: %d\n", p1.GetVisitedNodesCount())
		}

		if err != nil {
			var modelErr *modelchecker.ModelError
			if errors.As(err, &modelErr) {
				fmt.Println("Stack Trace:")
				fmt.Println(modelErr.SprintStackTrace())
			} else {
				fmt.Println("Error:", err)
			}
			os.Exit(1)
		}

		//fmt.Println("root", root)
		if failedNode == nil {
			var failurePath []*modelchecker.Link
			var failedInvariant *modelchecker.InvariantPosition
			nodes, messages, deadlock, yieldsCount := modelchecker.GetAllNodes(rootNode, stateConfig.GetOptions().GetMaxActions())

			if len(messages) > 0 && !simulation {
				graphDot := modelchecker.GenerateCommunicationGraph(messages)
				dotFileName := filepath.Join(outDir, "communication.dot")
				// Write the content to the file
				err := os.WriteFile(dotFileName, []byte(graphDot), 0644)
				if err != nil {
					fmt.Println("Error writing to file:", err)
					return
				}
				if !isPlayground {
					fmt.Printf("Writen communication diagram dotfile: %s\nTo generate svg, run: \n"+
						"dot -Tsvg %s -o communication.svg && open communication.svg\n", dotFileName, dotFileName)
				}
			}

			if deadlock != nil && stateConfig.GetDeadlockDetection() && !p1.Stopped() && !simulation {
				fmt.Println("DEADLOCK detected")
				fmt.Println("FAILED: Model checker failed")
				if simulation {
					fmt.Println("seed:", p1.Seed)
				}
				dumpFailedNode(sourceFileName, deadlock, rootNode, outDir)
				return
			}
			if !simulation {
				fmt.Println("Valid nodes:", len(nodes), "Unique states:", yieldsCount)
				invariants := modelchecker.CheckSimpleExistsWitness(nodes)
				if len(invariants) > 0 {
					fmt.Println("\nFAILED: Expected states never reached")
					for i2, invariant := range invariants {
						fmt.Printf("Invariant %d: %s\n", i2, f.Invariants[invariant.InvariantIndex].Name)
					}
					fmt.Println("Time taken to check invariant: ", time.Now().Sub(endTime))
					return
				}
			}
			if !simulation && !p1.Stopped() {
				if stateConfig.GetLiveness() == "" || stateConfig.GetLiveness() == "enabled" || stateConfig.GetLiveness() == "true" || stateConfig.GetLiveness() == "strict" || stateConfig.GetLiveness() == "strict/bfs" {
					failurePath, failedInvariant = modelchecker.CheckStrictLiveness(rootNode)
				} else if stateConfig.GetLiveness() == "eventual" || stateConfig.GetLiveness() == "nondeterministic" {
					failurePath, failedInvariant = modelchecker.CheckFastLiveness(nodes)
				}
				fmt.Printf("IsLive: %t\n", failedInvariant == nil)
				fmt.Printf("Time taken to check liveness: %v\n", time.Now().Sub(endTime))
			}

			if failedInvariant == nil && !simulation {
				fmt.Println("PASSED: Model checker completed successfully")
				//nodes, _, _ := modelchecker.GetAllNodes(rootNode)
				if saveStates || !isPlayground {
					nodeFiles, linkFileNames, err := modelchecker.GenerateProtoOfJson(nodes, outDir+"/")
					if err != nil {
						fmt.Println("Error generating proto files:", err)
						return
					}
					fmt.Printf("Writen %d node files and %d link files to dir %s\n", len(nodeFiles), len(linkFileNames), outDir)
				}
				return
			} else if failedInvariant != nil {
				fmt.Println("FAILED: Liveness check failed")
				if failedInvariant.FileIndex > 0 {
					fmt.Printf("Only one file expected. Got %d\n", failedInvariant.FileIndex)
				} else {
					fmt.Printf("Invariant: %s\n", f.Invariants[failedInvariant.InvariantIndex].Name)
				}
				GenerateFailurePath(sourceFileName, failurePath, failedInvariant, outDir)
				_, _, err = modelchecker.GenerateErrorPathProtoOfJson(failurePath, outDir+"/")
				if err != nil {
					fmt.Println("Error writing files", err)
				}
				return
			}

		} else if failedNode != nil {
			if failedNode.FailedInvariants != nil && len(failedNode.FailedInvariants) > 0 && len(failedNode.FailedInvariants[0]) > 0 {
				fmt.Println("FAILED: Model checker failed. Invariant: ", f.Invariants[failedNode.FailedInvariants[0][0]].Name)
			} else if simulation {
				fmt.Println("FAILED: Model checker failed. Deadlock/stuttering detected")
			}
			if simulation {
				fmt.Println("seed:", p1.Seed)
			}
			dumpFailedNode(sourceFileName, failedNode, rootNode, outDir)
			return
		}
	}
	fmt.Println("Stopped after", runs, "runs at ", time.Now())
}

func startModelChecker(err error, p1 *modelchecker.Processor) (*modelchecker.Node, *modelchecker.Node, time.Time) {
	if simulation {
		rootNode, failedNode, _ := p1.Start()
		return rootNode, failedNode, time.Now()
	}
	if internalProfile {
		startCpuProfile()
		defer pprof.StopCPUProfile()
	}
	startTime := time.Now()
	rootNode, failedNode, err := p1.Start()
	endTime := time.Now()
	fmt.Printf("Time taken for model checking: %v\n", endTime.Sub(startTime))
	if internalProfile {
		startHeapProfile()
	}
	return rootNode, failedNode, endTime
}

func startCpuProfile() {
	// Start CPU profiling
	f, err := os.Create("cpu.pprof")
	if err != nil {
		panic(err)
	}
	err = pprof.StartCPUProfile(f)
	if err != nil {
		panic(err)
	}
}

func startHeapProfile() {
	f, err := os.Create("mem.pprof")
	if err != nil {
		panic(err)
	}
	err = pprof.WriteHeapProfile(f)
	if err != nil {
		panic(err)
	}
}

func dumpFailedNode(srcFileName string, failedNode *modelchecker.Node, rootNode *modelchecker.Node, outDir string) {
	failurePath := make([]*modelchecker.Link, 0)
	node := failedNode
	for node != nil {

		if len(node.Inbound) == 0 || node.Name == "init" || node == rootNode {
			link := modelchecker.InitNodeToLink(node)
			failurePath = append(failurePath, link)
			break
		}
		outLink := modelchecker.ReverseLink(node, node.Inbound[0])
		failurePath = append(failurePath, outLink)
		//node.Name = node.Name + "/" + node.Inbound[0].Name
		node = node.Inbound[0].Node
	}
	slices.Reverse(failurePath)
	GenerateFailurePath(srcFileName, failurePath, nil, outDir)
	_, _, err := modelchecker.GenerateErrorPathProtoOfJson(failurePath, outDir+"/")
	if err != nil {
		fmt.Println("Error writing files", err)
	}
}

func GenerateFailurePath(srcFileName string, failurePath []*modelchecker.Link, invariant *modelchecker.InvariantPosition, outDir string) {
	for _, link := range failurePath {
		node := link.Node
		stepName := link.Name

		fmt.Printf("------\n%s\n", stepName)

		nodeStr := node.Heap.ToJson()
		nodeStr = strings.ReplaceAll(nodeStr, lib.SymmetryPrefix, "")
		fmt.Printf("--\nstate: %s\n", nodeStr)
		if len(node.Returns) > 0 {
			fmt.Printf("returns: %s\n", strings.ReplaceAll(node.Returns.String(), lib.SymmetryPrefix, ""))
		}
	}
	fmt.Println("------")
	if !isPlayground {
		errJsonFileName := filepath.Join(outDir, "error-graph.json")
		bytes, err := json.MarshalIndent(failurePath, "", "  ")
		if err != nil {
			fmt.Println("Error creating json:", err)
		}
		err = os.WriteFile(errJsonFileName, bytes, 0644)
		if err != nil {
			fmt.Println("Error writing to file:", err)
			return
		}
		fmt.Printf("Writen graph json: %s\n", errJsonFileName)
	}

	dotStr := modelchecker.GenerateFailurePath(failurePath, invariant)
	//fmt.Println(dotStr)
	dotFileName := filepath.Join(outDir, "error-graph.dot")
	// Write the content to the file
	err := os.WriteFile(dotFileName, []byte(dotStr), 0644)
	if err != nil {
		fmt.Println("Error writing to file:", err)
		return
	}
	if !isPlayground {
		fmt.Printf("Writen graph dotfile: %s\nTo generate an image file, run: \n"+
			"dot -Tsvg %s -o error-graph.svg && open error-graph.svg\n", dotFileName, dotFileName)
	}
	err = GenerateFailurePathHtml(srcFileName, failurePath, invariant, outDir)
	if err != nil {
		return
	}
	if !isPlayground {
		fmt.Printf("Writen error states as html: %s/error-states.html\nTo open: \n"+
			"open %s/error-states.html\n", outDir, outDir)
	}
}

func createOutputDir(dirPath string) (string, error) {
    // Create the directory name with current date and time
    dateTimeStr := time.Now().Format("2006-01-02_15-04-05") // Format: YYYY-MM-DD_HH-MM-SS
    newDirName := fmt.Sprintf("run_%s", dateTimeStr)

    // Create the full path for the new directory
    newDirPath := filepath.Join(dirPath, "out", newDirName)

    // Create the directory
    if err := os.MkdirAll(newDirPath, 0755); err != nil {
        fmt.Println("Error creating directory:", err)
        return newDirPath, err
    }

    // Define the symlink path
    latestSymlinkPath := filepath.Join(dirPath, "out", "latest")

    // Remove the existing symlink if it exists
    if _, err := os.Lstat(latestSymlinkPath); err == nil {
        if err := os.Remove(latestSymlinkPath); err != nil {
            fmt.Println("Error removing existing symlink:", err)
            return newDirPath, err
        }
    }
    // Convert to absolute path
    absNewDirPath, err := filepath.Abs(newDirPath)
    if err != nil {
        fmt.Println("Error resolving absolute path:", err)
        return "", err
    }
    // Create the new symlink
    if err := os.Symlink(absNewDirPath, latestSymlinkPath); err != nil {
        fmt.Println("Error creating symlink:", err)
        return newDirPath, err
    }
    // Still returning the newDirPath instead of the symlink path
    // So, all the output logs will still point to the newDirPath.
    // This reduces issues when multiple executions are run in parallel.
    return newDirPath, nil
}

// Helper to remove Messages and Labels from the Links
func removeFields(link *modelchecker.Link) *modelchecker.Link {
	linkCopy := *link
	linkCopy.Messages = nil
	linkCopy.Labels = nil
	return &linkCopy
}

// Helper to convert a Link to base64-encoded JSON without Messages and Labels
func linkToBase64(link *modelchecker.Link) (string, error) {
	linkCopy := removeFields(link)
	jsonBytes, err := json.Marshal(linkCopy)
	if err != nil {
		return "", err
	}
	str := string(jsonBytes)
	str = strings.ReplaceAll(str, lib.SymmetryPrefix, "")
	return base64.StdEncoding.EncodeToString([]byte(str)), nil
}

// Helper to create the JSON diff URL
func createDiffURL(leftBase64, rightBase64 string) string {
	return fmt.Sprintf("https://jsondiff.com/#left=data:base64,%s&right=data:base64,%s", leftBase64, rightBase64)
}

// Helper to write a single row in the HTML table
func writeRow(tmpl *template.Template, file *os.File, rowNum, lineNum int, name string, lane, maxLanes int, nodeName, diffURL, yieldDiffURL string) error {
	if nodeName != "yield" {
		nodeName = ""
	}
	contentStr := name
	if lineNum > 0 {
		contentStr = fmt.Sprintf("%s<br><p id=\"line-%d-ref\" class=\"line-num\">Next Instr: %d<p>", name, lineNum, lineNum)
	}
	content := template.HTML(contentStr)
	lanes := make([]template.HTML, maxLanes)
	for i := 0; i < maxLanes; i++ {
		if i == lane {
			lanes[i] = content // Replace with actual value for this lane
		} else {
			lanes[i] = "" // Empty for the other lanes
		}
	}
	data := map[string]interface{}{
		"RowNum":       rowNum,
		"Name":         name,
		"Lanes":        lanes,
		"NodeName":     nodeName,
		"DiffURL":      diffURL,
		"YieldDiffURL": yieldDiffURL,
	}

	return tmpl.Execute(file, data)
}

// Template for HTML generation
const htmlTemplate = `
<tr>
	<td>{{.RowNum}}</td>
	{{range .Lanes}} 
		<td>{{.}}</td>
	{{else}}
		<td></td>  <!-- Empty column if no lane is filled -->
	{{end}}
	<td>{{.NodeName}}</td>
	<td style="min-width:6em; text-align:center;">{{if .DiffURL}}<a href="{{.DiffURL}}" target="_blank">Show diff</a>{{end}}</td>
	<td style="min-width:6em; text-align:center;">{{if .YieldDiffURL}}<a href="{{.YieldDiffURL}}" target="_blank">Show yield diff</a>{{end}}</td>
</tr>
`

// GenerateFailurePathHtml creates an HTML file showing differences between adjacent failurePath Links
func GenerateFailurePathHtml(srcFileName string, failurePath []*modelchecker.Link, invariant *modelchecker.InvariantPosition, outDir string) error {
	// Create the output file in the specified directory
	outputFilePath := filepath.Join(outDir, "error-states.html")
	file, err := os.Create(outputFilePath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()
	maxLanes := 0
	for _, link := range failurePath {
		if link.ReqId+1 > maxLanes {
			maxLanes = link.ReqId + 1
		}
	}

	// Start writing the HTML file
	file.WriteString(`
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Error States Comparison</title>
</head>
<style>
/* styles.css */

* {
  box-sizing: border-box;
  margin: 0;
  padding: 0;
}

body, html {
  height: 100%;
  font-family: Arial, sans-serif;
}

.container {
  display: flex;
  height: 100vh;
  padding: 10px;
}

.content {
  flex: 1;
  padding-right: 20px;
}
.content td, .content th {
  padding: 4px 8px;
}
.code-container {
  width: 50%;
  position: relative;
  overflow: hidden;
}

.code {
  counter-reset: lineNumber;
  overflow-y: auto;
  height: 100%;
  white-space: nowrap;
  padding-top: 10px;
}

.code-line-numbers {
  position: absolute;
  top: 0;
  left: 0;
  padding-top: 10px;
  padding-right: 10px;
  text-align: right;
  font-family: monospace;
  background-color: #f4f4f4;
  color: #888;
  user-select: none;
}
.line-number {
  line-height: 1.6;
}
.line-num:hover {
    cursor: pointer;
}
.code pre {
  display: flex;
  margin: 0;
  padding: 0;
  font-family: monospace;
  line-height: 1.6;
  padding-left: 3em;
  position: relative;
}
.code pre:before {
    counter-increment: lineNumber;
    content: counter(lineNumber) " ";
    position: absolute;
    left: 0;
    top: 0;
    width: 2.5em;
    text-align: right;
    color: #888;
    background-color: #f4f4f4;
    user-select: none;
    font-family: monospace;
}

code {
  display: block;
}

.highlight {
  background-color: yellow;
  animation: highlight 1s ease-out;
}

@keyframes highlight {
  0% {
    background-color: yellow;
  }
  100% {
    background-color: transparent;
  }
}

</style>
<body>
<div class="container">
  <!-- Main content area (left) -->
  <div class="content">
    <h1>Error States Diff</h1>
    <table border="1">
        <tr>
            <th>Row</th>`)
	for i := 0; i < maxLanes; i++ {
		file.WriteString(fmt.Sprintf("<th>Thread %d</th>", i))
	}

	file.WriteString(`
            <th>Yield?</th>
            <th style="min-width:6em; text-align:center;">Diff Link</th>
            <th style="min-width:6em; text-align:center;">Yield Diff</th>
        </tr>
`)

	// Initialize template for rows
	tmpl, err := template.New("row").Parse(htmlTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	var lastYieldObj *modelchecker.Link

	lane := 0

	// Process the first element (0th object) separately
	if len(failurePath) > 0 {
		firstLink := failurePath[0]
		lineNum := getLineNumber(firstLink)
		writeRow(tmpl, file, 1, lineNum, firstLink.Name, lane, maxLanes, firstLink.Node.Name, "", "")
		if firstLink.Node.Name == "yield" {
			lastYieldObj = firstLink
		}

		lane = firstLink.ReqId
	}

	// Iterate through remaining pairs
	for i := 1; i < len(failurePath); i++ {
		leftLink := failurePath[i-1]
		rightLink := failurePath[i]
		lane = rightLink.ReqId
		lineNum := getLineNumber(rightLink)

		// Convert both Links to base64 JSON
		leftBase64, err := linkToBase64(leftLink)
		if err != nil {
			return fmt.Errorf("failed to encode left link to base64: %w", err)
		}

		rightBase64, err := linkToBase64(rightLink)
		if err != nil {
			return fmt.Errorf("failed to encode right link to base64: %w", err)
		}

		// Create the JSON diff URL
		diffURL := createDiffURL(leftBase64, rightBase64)

		// Check if Node.Name == "yield" for this object
		yieldDiffURL := ""
		if rightLink.Node.Name == "yield" && lastYieldObj != nil {
			// Create a yield diff link between this and the last "yield" object
			lastYieldBase64, err := linkToBase64(lastYieldObj)
			if err != nil {
				return fmt.Errorf("failed to encode last yield link to base64: %w", err)
			}
			yieldDiffURL = createDiffURL(lastYieldBase64, rightBase64)
		}

		// Update last yield object and index if current Node.Name is "yield"
		if rightLink.Node.Name == "yield" {
			lastYieldObj = rightLink
		}

		// Write the row to the HTML file
		writeRow(tmpl, file, i+1, lineNum, rightLink.Name, lane, maxLanes, rightLink.Node.Name, diffURL, yieldDiffURL)
	}

	// Close the table and HTML file
	file.WriteString(`
    </table>
  </div>
    <!-- Code block area (right) -->
    <div class="code-container">
      <div class="code-line-numbers" id="line-numbers">
        <!-- Line numbers will be added dynamically here -->
      </div>
      <div class="code" id="code">`)
	srcFileBytes, err := os.ReadFile(srcFileName)
	srcFileString := ""
	if err != nil {
		fmt.Println("Error reading source file:", err)
	} else {
		srcFileString = string(srcFileBytes)
	}
	lines := strings.Split(srcFileString, "\n")
	for _, line := range lines {
		escapedString := html.EscapeString(line)
		if strings.TrimSpace(escapedString) == "" {
			escapedString = "&nbsp;"
		}
		file.WriteString(fmt.Sprintf("<pre><code>%s</code></pre>\n", escapedString))
	}
	//file.WriteString(`
	//    <!-- Code lines will be added here -->
	//    <pre><code>def my_function():</code></pre>
	//    <pre><code>    print("Hello World")</code></pre>
	//    <pre><code>    return True</code></pre>
	//    <pre><code>def another_function():</code></pre>
	//    <pre><code>    print("This is another function")</code></pre>
	//    <pre><code>    return False</code></pre>
	//    <!-- Add more lines of code as needed -->`)
	file.WriteString(`
      </div>
    </div>
</div>
</body>
<script>
// script.js

document.addEventListener("DOMContentLoaded", function() {
  const codeLines = document.querySelectorAll("#code pre");
  //const lineNumbers = document.getElementById("line-numbers");
  const codeDiv = document.getElementById("code");
  
  //// Function to create line numbers and assign them as clickable
  //codeLines.forEach((line, index) => {
  //  const lineNumber = document.createElement("div");
  //  lineNumber.textContent = index + 1; // Line numbers are 1-indexed
  //  lineNumber.classList.add("line-number");
  //  lineNumber.addEventListener("click", () => highlightLine(index));
  //  lineNumbers.appendChild(lineNumber);
  //});

  // Function to highlight a line in the code block
  function highlightLine(lineIndex) {
    // Remove existing highlight
    const highlighted = codeDiv.querySelector(".highlight");
    if (highlighted) {
      highlighted.classList.remove("highlight");
    }
  
    // Add highlight to clicked line
    const targetLine = codeLines[lineIndex];
    if (!targetLine) return;
    targetLine.classList.add("highlight");
  
    // Scroll the code block to the target line
    targetLine.scrollIntoView({ behavior: "auto",  block: "center" });
  }

  // Scroll the code block to make a specific line visible when a reference in content is clicked
  document.querySelectorAll("[id^='line-']").forEach(element => {
    element.addEventListener("click", function() {
      const lineNumber = parseInt(this.id.split('-')[1], 10) - 1; // Get line number from ID
      highlightLine(lineNumber);
    });
  });
});

</script>
</html>
`)

	return nil
}

func getLineNumber(link *modelchecker.Link) int {
	if link.Node == nil || link.Node.Process == nil || len(link.Node.Threads) <= link.ReqId || link.Node.Threads[link.ReqId] == nil {
		return 0
	}
	sourceInfo := link.Node.Threads[link.ReqId].CurrentPcSourceInfo()
	return int(sourceInfo.GetStart().GetLine())
}
