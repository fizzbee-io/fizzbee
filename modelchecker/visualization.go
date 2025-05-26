package modelchecker

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/fizzbee-io/fizzbee/lib"
	"html"
	"html/template"
	"os"
	"path/filepath"
	"strings"
)

// Helper to remove Messages and Labels from the Links
func removeFields(link *Link) *Link {
	linkCopy := *link
	linkCopy.Messages = nil
	linkCopy.Labels = nil
	return &linkCopy
}

// Helper to convert a Link to base64-encoded JSON without Messages and Labels
func linkToBase64(link *Link) (string, error) {
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
func GenerateFailurePathHtml(srcFileName string, failurePath []*Link, invariant *InvariantPosition, outDir string) error {
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

	var lastYieldObj *Link

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

func getLineNumber(link *Link) int {
	if link.Node == nil || link.Node.Process == nil || len(link.Node.Threads) <= link.ReqId || link.Node.Threads[link.ReqId] == nil {
		return 0
	}
	sourceInfo := link.Node.Threads[link.ReqId].CurrentPcSourceInfo()
	return int(sourceInfo.GetStart().GetLine())
}
