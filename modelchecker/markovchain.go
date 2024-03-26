package modelchecker

import (
	"fizz/proto"
	"fmt"
	"github.com/jayaprabhakar/fizzbee/lib"
	"math"
)

func matrixVectorProduct(matrix [][]float64, vector []float64) []float64 {
	result := make([]float64, len(vector))

	for i := range matrix {
		for j := range matrix[i] {
			result[i] += matrix[i][j] * vector[j]
		}
	}
	//fmt.Printf("Matrix Vector Product:%v\n", result)
	return result
}

func multiplyMatrices(a, b [][]float64) [][]float64 {
	rows := len(a)
	cols := len(a[0])

	// Check if matrices have the same dimensions
	if len(b) != rows || len(b[0]) != cols {
		panic("Matrices must have the same dimensions for element-wise multiplication")
	}

	// Initialize the result matrix
	result := make([][]float64, rows)
	for i := range result {
		result[i] = make([]float64, cols)
	}

	// Perform element-wise multiplication
	for i := 0; i < rows; i++ {
		for j := 0; j < cols; j++ {
			result[i][j] = a[i][j] * b[i][j]
		}
	}

	return result
}

func vectorNorm(vector []float64) float64 {
	sum := 0.0
	for _, v := range vector {
		sum += v * v
		//sum += v
	}
	//return sum / float64(len(vector))
	return math.Sqrt(sum)
}

func normalizeVector(vector []float64) {
	norm := vectorNorm(vector)
	for i := range vector {
		vector[i] /= norm
	}
}
func printMatrix(matrix [][]float64) {
	fmt.Println("[")
	for _, row := range matrix {
		fmt.Print("[")
		for _, v := range row {
			fmt.Printf("%f,", v)
		}
		fmt.Println("],")
	}
	fmt.Println("]")
}

type Histogram struct {
	entries []HistogramEntry
	mean map[string]float64
	variance map[string]float64
}

func (h *Histogram) GetMeanCounts() map[string]float64 {
	return h.mean
}

func (h *Histogram) GetAllHistogram() []HistogramEntry {
	return h.entries
}

func (h *Histogram) GetMean(counter string) float64 {
	return h.mean[counter]
}

type HistogramEntry struct {
	percentile float64
	counters  map[string]float64
}

func (h *Histogram) addEntry(percentile float64, counters map[string]float64) {
	newCounters := make(map[string]float64)
	for k, v := range counters {
		newCounters[k] = v
	}
	h.entries = append(h.entries, HistogramEntry{percentile: percentile, counters: newCounters})
}

func newHistogram() *Histogram {
	return &Histogram{entries: make([]HistogramEntry, 0), mean: make(map[string]float64), variance: make(map[string]float64)}
}

func steadyStateDistribution(root *Node, perfModel *proto.PerformanceModel) ([]float64, *Histogram) {


	// Create the transition matrix
	nodes, _, _ := getAllNodes(root)
	initialDistribution := make([]float64, len(nodes))
	initialDistribution[0] = 1.0 // Start from the root node
	

	//for i, node := range nodes {
	//	if node.Process == nil {
	//		continue
	//	}
	//	fmt.Printf("%d: %s\n", i, node.Heap.String())
	//}

	//transitionMatrix := createTransitionMatrix(nodes)
	transitionMatrix := genTransitionMatrix(nodes, perfModel)
	return markovChainAnalysis(nodes, perfModel, transitionMatrix, initialDistribution)
}

func markovChainAnalysis(nodes []*Node, perfModel *proto.PerformanceModel, transitionMatrix [][]float64, initialDistribution []float64) ([]float64, *Histogram) {
	matrices := genCounterMatrices(nodes, perfModel)
	histogram := newHistogram()
	//fmt.Printf("\ninitial distribution:\n%v\n", initialDistribution)
	//fmt.Printf("\nTransition Matrix:\n%v\n", transitionMatrix)
	transitionMatrix = transpose(transitionMatrix)
	//printMatrix(transitionMatrix)
	expectedCounterMatrices := make(map[string][][]float64)
	//rawCounterMatrices := make(map[string][][]float64)
	mean := make(map[string]float64)
	rawCounters := make(map[string]float64)
	for counterName, matrix := range matrices {
		m := transpose(matrix)
		expectedCounterMatrices[counterName] = multiplyMatrices(m, transitionMatrix)
		//rawCounterMatrices[counterName] = m
		mean[counterName] = 0.0
		rawCounters[counterName] = 0.0
		//fmt.Println(counterName)
		//printMatrix(expectedCounterMatrices[counterName])
	}

	// Compute the matrix power (raise the matrix to a sufficiently large power)
	iterations := 10000

	// Iterate to find the steady-state distribution
	currentDistribution := initialDistribution
	//fmt.Println(currentDistribution)
	altCurrentDistribution := make([]float64, len(nodes))
	copy(altCurrentDistribution, currentDistribution)
	prevTerminationProbability := 0.0
	for i := 0; i < iterations; i++ { // Max iterations to avoid infinite loop
		terminationProbability := 0.0
		for counter, counterMatrix := range expectedCounterMatrices {
			mean[counter] += sum(matrixVectorProduct(counterMatrix, currentDistribution))
			rawCounters[counter] += sum(matrixVectorProduct(counterMatrix, altCurrentDistribution))
		}

		nextDistribution := matrixVectorProduct(transitionMatrix, currentDistribution)
		altCurrentDistribution = matrixVectorProduct(transitionMatrix, altCurrentDistribution)

		//fmt.Println(i+1, nextDistribution)


		totalProb := 0.0
		for j, _ := range altCurrentDistribution {
			if transitionMatrix[j][j] == 1.0 || (nodes[j].Process != nil &&
				len(nodes[j].Process.Threads) == 0 && len(nodes[j].Process.Witness) > 0 && len(nodes[j].Process.Witness[0]) > 0 &&
				nodes[j].Process.Witness[0][0]) {
				altCurrentDistribution[j] = 0.0
				terminationProbability += nextDistribution[j]
			}
			totalProb += altCurrentDistribution[j]

		}
		if len(mean) > 0 {
			//fmt.Println(i+1, rawCounters)
			//fmt.Println(i+1, mean)
			//fmt.Println(i+1, terminationProbability)
			if terminationProbability > prevTerminationProbability {
				prevTerminationProbability = terminationProbability
				histogram.addEntry(terminationProbability, rawCounters)
			}
		}
		for j, f := range altCurrentDistribution {
			altCurrentDistribution[j] = f / totalProb
		}
		//fmt.Println(i+1, terminationProbability)
		// Check for convergence (you may define a suitable threshold)
		if vectorNorm(vectorDifference(nextDistribution, currentDistribution)) < 1e-7 {
			break
		}

		currentDistribution = nextDistribution
	}
	//fmt.Println(mean)
	//fmt.Println(rawCounters)
	histogram.mean = mean
	return currentDistribution, histogram
}

func FindAbsorptionCosts(root *Node, perfModel *proto.PerformanceModel, fileId int, invariantId int) ([]float64, *Histogram) {
	// Create the transition matrix
	nodes, _, yields := getAllNodes(root)
	//fmt.Println("Yields", yields)
	yields += 1 // Add the root node

	transitionMatrix := createAbsorptionTransitionMatrix(nodes, fileId, invariantId)
	//printMatrix(transitionMatrix)
	initialDistribution := make([]float64, len(nodes))
	for i, _ := range initialDistribution {
		if nodes[i].Name == "init" || nodes[i].Name == "yield" {
			initialDistribution[i] = 1.0 / float64(yields) // Set every node to 1.0/n
		}
	}
	steadstate, histogram := markovChainAnalysis(nodes, perfModel, transitionMatrix, initialDistribution)
	//fmt.Println("liveness ", steadstate)
	fmt.Println("liveness mean counts", histogram.GetMeanCounts())
	fmt.Println("liveness histogram", histogram.GetAllHistogram())
	return steadstate, histogram
}

func createAbsorptionTransitionMatrix(nodes []*Node, fileId int, invariantId int) [][]float64 {
	transitionMatrix := createTransitionMatrix(nodes)
	//fmt.Printf("\nTransition Matrix:\n%v\n", transitionMatrix)

	//printMatrix(transitionMatrix)
	for i, matrix := range transitionMatrix {
		if nodes[i].Process == nil {
			continue
		}
		if nodes[i].Witness[fileId][invariantId] {
			for j := range matrix {
				if i == j {
					matrix[j] = 1.0
				} else {
					matrix[j] = 0.0
				}
			}
		}
	}
	//transitionMatrix = transpose(transitionMatrix)
	//printMatrix(transitionMatrix)
	//transitionMatrix = normalizeColumns(transitionMatrix)
	transitionMatrix = normalizeRows(transitionMatrix)
	return transitionMatrix
}


func checkLivenessAndCost(root *Node, perfModel *proto.PerformanceModel, fileId int, invariantId int) ([]float64, *Histogram) {
	// Create the transition matrix
	nodes, _, yields := getAllNodes(root)
	fmt.Println("Yields", yields)
	yields += 1 // Add the root node

	transitionMatrix := createAbsorptionTransitionMatrix(nodes, fileId, invariantId)
	//printMatrix(transitionMatrix)
	initialDistribution := make([]float64, len(nodes))
	for i, _ := range initialDistribution {
		if nodes[i].Name == "init" || nodes[i].Name == "yield" {
			initialDistribution[i] = 1.0 / float64(yields) // Set every node to 1.0/n
		}
	}
	steadstate, histogram := markovChainAnalysis(nodes, perfModel, transitionMatrix, initialDistribution)
	//fmt.Println("liveness ", steadstate)
	fmt.Println("liveness mean counts", histogram.GetMeanCounts())
	fmt.Println("liveness histogram", histogram.GetAllHistogram())
	return steadstate, histogram
}

func checkLiveness(root *Node, fileId int, invariantId int) []float64 {
	// Create the transition matrix
	nodes, _, _ := getAllNodes(root)

	transitionMatrix := createTransitionMatrix(nodes)
	//fmt.Printf("\nTransition Matrix:\n%v\n", transitionMatrix)


	//printMatrix(transitionMatrix)
	for i, matrix := range transitionMatrix {
		if nodes[i].Process == nil {
			continue
		}
		if nodes[i].Witness[fileId][invariantId] {
			for j := range matrix {
				if i == j {
					matrix[j] = 1.0
				} else {
					matrix[j] = 0.0
				}
			}
		}
	}
	transitionMatrix = transpose(transitionMatrix)
	//printMatrix(transitionMatrix)
	transitionMatrix = normalizeColumns(transitionMatrix)
	//printMatrix(transitionMatrix)

	// Compute the matrix power (raise the matrix to a sufficiently large power)
	iterations := 2000

	initialDistribution := make([]float64, len(nodes))
	for i, _ := range initialDistribution {
		initialDistribution[i] = 1.0 / float64(len(nodes)) // Set every node to 1.0/n
	}

	// Iterate to find the steady-state distribution
	currentDistribution := initialDistribution
	//fmt.Println(currentDistribution)
	for i := 0; i < iterations; i++ { // Max iterations to avoid infinite loop
		nextDistribution := matrixVectorProduct(transitionMatrix, currentDistribution)
		//fmt.Println(i, nextDistribution)
		// Check for convergence (you may define a suitable threshold)
		if vectorNorm(vectorDifference(nextDistribution, currentDistribution)) < 1e-7 {
			break
		}

		currentDistribution = nextDistribution
	}

	return currentDistribution
}
func normalizeRows(matrix [][]float64) [][]float64 {

	// Iterate over each column
	for _, row := range matrix {
		// Calculate the sum of values in the column
		rowSum := 0.0
		for _, val := range row {
			rowSum += val
		}

		// Normalize the values in the column
		if rowSum != 0 {
			for columnIndex := range row {
				row[columnIndex] /= rowSum
			}
		}
	}

	return matrix
}
func normalizeColumns(matrix [][]float64) [][]float64 {
	// Get the number of columns
	numColumns := len(matrix[0])

	// Iterate over each column
	for col := 0; col < numColumns; col++ {
		// Calculate the sum of values in the column
		columnSum := 0.0
		for _, row := range matrix {
			columnSum += row[col]
		}

		// Normalize the values in the column
		if columnSum != 0 {
			for row := range matrix {
				matrix[row][col] /= columnSum
			}
		}
	}

	return matrix
}

func sum(distribution []float64) float64 {
	sum := 0.0
	for _, v := range distribution {
		sum += v
	}
	return sum
}
func vectorDifference(a, b []float64) []float64 {
	result := make([]float64, len(a))
	for i := range a {
		result[i] = a[i] - b[i]
	}
	return result
}

func createTransitionMatrix(nodes []*Node) [][]float64 {
	n := len(nodes)
	matrix := make([][]float64, n)
	for i := range matrix {
		matrix[i] = make([]float64, n)
	}

	indexMap := make(map[*Node]int)
	for i, node := range nodes {
		indexMap[node] = i
	}

	for _, node := range nodes {
		if len(node.Outbound) == 0 {
			matrix[indexMap[node]][indexMap[node]] = 1.0
		}
		for _, outboundNode := range node.Outbound {
			matrix[indexMap[node]][indexMap[outboundNode.Node]] += 1.0 / float64(len(node.Outbound))
		}

	}

	return matrix
}
func transpose(matrix [][]float64) [][]float64 {
	rows := len(matrix)
	cols := len(matrix[0])

	result := make([][]float64, cols)
	for i := range result {
		result[i] = make([]float64, rows)
	}

	for i := 0; i < rows; i++ {
		for j := 0; j < cols; j++ {
			result[j][i] = matrix[i][j]
		}
	}

	return result
}

func GetAllNodes(root *Node) ([]*Node, *Node, int) {
	return getAllNodes(root)

}
func getAllNodes(root *Node) ([]*Node, *Node, int) {
	// Implement a traversal to get all nodes in the graph
	// This can be a simple depth-first or breadth-first traversal
	// depending on your requirements and graph structure.
	// For simplicity, let's assume a simple depth-first traversal here.

	result, deadlock, yield := traverseBFS(root)
	//visited := make(map[*Node]bool)
	//var result []*Node
	//yield := 0
	//maxDepth := 0
	//traverseDFS(root, visited, &result, &yield, &maxDepth)
	//fmt.Println("Max Depth", maxDepth)
	return result, deadlock, yield
}

func traverseDFS(node *Node, visited map[*Node]bool, result *[]*Node, yield *int, maxDepth *int) {
	if node == nil || visited[node] {
		return
	}

	visited[node] = true

	if node.Process != nil && !node.Process.Enabled {
		return
	}

	*result = append(*result, node)
	if node.Name == "yield" {
		*yield++
	}
	if node.forkDepth > *maxDepth {
		*maxDepth = node.forkDepth
	}

	enabledLinks := make([]*Link, 0)
	for _, link := range node.Outbound {
		if !link.Node.Enabled {
			continue
		}
		enabledLinks = append(enabledLinks, link)
	}
	node.Outbound = enabledLinks

	for _, outboundNode := range node.Outbound {
		traverseDFS(outboundNode.Node, visited, result, yield, maxDepth)
	}
}

func traverseBFS(rootNode *Node) ([]*Node, *Node, int) {
	var deadlock *Node
	visited := make(map[*Node]bool)
	var result []*Node
	queue := lib.NewQueue[*Node]()
	queue.Enqueue(rootNode)
	yield := 0
	maxDepth := 0
	for queue.Count() > 0 {
		node, _ := queue.Dequeue()

		if visited[node] {
			continue
		}
		visited[node] = true

		if node.Process != nil && !node.Process.Enabled {
			continue
		}

		result = append(result, node)
		if node.Name == "yield" {
			yield++
		}
		if node.forkDepth > maxDepth {
			maxDepth = node.forkDepth
		}
		enabledLinks := make([]*Link, 0)
		for _, link := range node.Outbound {
			if !link.Node.Enabled {
				continue
			}
			enabledLinks = append(enabledLinks, link)
		}
		node.Outbound = enabledLinks
		if len(enabledLinks) == 0 && deadlock == nil {
			deadlock = node
		}

		for _, link := range node.Outbound {
			queue.Enqueue(link.Node)
		}
	}
	fmt.Println("Max Depth", maxDepth)
	return result, deadlock, yield
}