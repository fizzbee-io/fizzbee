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
func main() {
    flag.BoolVar(&isPlayground, "playground", false, "is for playground")
    flag.BoolVar(&simulation, "simulation", false, "Runs in simulation mode (DFS). Default=false for no simulation (BFS)")
    flag.BoolVar(&internalProfile, "internal_profile", false, "Enables CPU and memory profiling of the model checker")
    flag.BoolVar(&saveStates, "save_states", false, "Save states to disk")
    flag.Int64Var(&seed, "seed", 0, "Seed for random number generator used in simulation mode")
    flag.IntVar(&maxRuns, "max_runs", 0, "Maximum number of simulation runs/paths to explore. Default=0 for unlimited")
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
                    Options: &ast.Options{MaxActions: 100, MaxConcurrentActions: 2, CrashOnYield: &crashOnYield},
                    Liveness: "strict",
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

        p1 = modelchecker.NewProcessor([]*ast.File{f}, stateConfig, simulation, seed, dirPath)
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
                fmt.Printf("Writen graph dotfile: %s\nTo generate svg, run: \n" +
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
                    fmt.Printf("Writen communication diagram dotfile: %s\nTo generate svg, run: \n" +
                        "dot -Tsvg %s -o communication.svg && open communication.svg\n", dotFileName, dotFileName)
                }
            }

            if deadlock != nil && stateConfig.GetDeadlockDetection() && !p1.Stopped() && !simulation {
                fmt.Println("DEADLOCK detected")
                fmt.Println("FAILED: Model checker failed")
                if simulation {
                    fmt.Println("seed:", p1.Seed)
                }
                dumpFailedNode(deadlock, rootNode, outDir)
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
                if stateConfig.GetLiveness() == "" || stateConfig.GetLiveness() == "enabled" || stateConfig.GetLiveness() == "true"  || stateConfig.GetLiveness() == "strict" || stateConfig.GetLiveness() == "strict/bfs" {
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
                GenerateFailurePath(failurePath, failedInvariant, outDir)
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
            dumpFailedNode(failedNode, rootNode, outDir)
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


func dumpFailedNode(failedNode *modelchecker.Node, rootNode *modelchecker.Node, outDir string) {
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
    GenerateFailurePath(failurePath, nil, outDir)
}

func GenerateFailurePath(failurePath []*modelchecker.Link, invariant *modelchecker.InvariantPosition, outDir string) {
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
    err = GenerateFailurePathHtml(failurePath, invariant, outDir)
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
    return base64.StdEncoding.EncodeToString(jsonBytes), nil
}

// Helper to create the JSON diff URL
func createDiffURL(leftBase64, rightBase64 string) string {
    return fmt.Sprintf("https://jsondiff.com/#left=data:base64,%s&right=data:base64,%s", leftBase64, rightBase64)
}

// Helper to write a single row in the HTML table
func writeRow(tmpl *template.Template, file *os.File, rowNum int, name, nodeName, diffURL, yieldDiffURL string) error {
    if name == nodeName {
        nodeName = ""
    }

    data := map[string]interface{}{
        "RowNum":       rowNum,
        "Name":         name,
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
	<td>{{.Name}}</td>
	<td>{{.NodeName}}</td>
	<td style="min-width:6em; text-align:center;">{{if .DiffURL}}<a href="{{.DiffURL}}" target="_blank">Show diff</a>{{end}}</td>
	<td style="min-width:6em; text-align:center;">{{if .YieldDiffURL}}<a href="{{.YieldDiffURL}}" target="_blank">Show yield diff</a>{{end}}</td>
</tr>
`

// GenerateFailurePathHtml creates an HTML file showing differences between adjacent failurePath Links
func GenerateFailurePathHtml(failurePath []*modelchecker.Link, invariant *modelchecker.InvariantPosition, outDir string) error {
    // Create the output file in the specified directory
    outputFilePath := filepath.Join(outDir, "error-states.html")
    file, err := os.Create(outputFilePath)
    if err != nil {
        return fmt.Errorf("failed to create file: %w", err)
    }
    defer file.Close()

    // Start writing the HTML file
    file.WriteString(`
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Error States Comparison</title>
</head>
<body>
    <h1>Error States Diff</h1>
    <table border="1">
        <tr>
            <th>Row</th>
            <th>Name</th>
            <th>Node.Name</th>
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

    // Process the first element (0th object) separately
    if len(failurePath) > 0 {
        firstLink := failurePath[0]
        writeRow(tmpl, file, 1, firstLink.Name, firstLink.Node.Name, "", "")
        if firstLink.Node.Name == "yield" {
            lastYieldObj = firstLink
        }
    }

    // Iterate through remaining pairs
    for i := 1; i < len(failurePath); i++ {
        leftLink := failurePath[i-1]
        rightLink := failurePath[i]

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
        writeRow(tmpl, file, i+1, rightLink.Name, rightLink.Node.Name, diffURL, yieldDiffURL)
    }

    // Close the table and HTML file
    file.WriteString(`
    </table>
</body>
</html>
`)

    return nil
}