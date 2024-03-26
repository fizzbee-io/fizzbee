package modelchecker

import (
	"fizz/proto"
	"fmt"
	proto2 "github.com/golang/protobuf/proto"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func GenerateProtoOfJson(nodes []*Node, pathPrefix string) ([]string, []string, error) {
	dir := filepath.Dir(pathPrefix)
	err := os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		fmt.Printf("Error creating directory %s: %v\n", dir, err)
		return nil, nil, err
	}

	shardSize := 10000
	n := len(nodes)
	shards := n / shardSize

	indexMap := make(map[*Node]int)
	filename := fmt.Sprintf("%snodes_%06d_of_%06d.pb", pathPrefix, 0, shards)
	nodeJsons := make([]string, 0, shardSize)
	jsonFileNames := make([]string, 0, shards)
	edges := 0
	for i, node := range nodes {
		indexMap[node] = i
		edges += max(1, len(node.Outbound))
		if len(nodeJsons) >= shardSize {
			err := writeNodeJsonsToFile(nodeJsons, filename)
			if err != nil {
				return nil, nil, err
			}
			jsonFileNames = append(jsonFileNames, filename)
			nodeJsons = nodeJsons[:0]
			filename = fmt.Sprintf("%snodes_%06d_of_%06d.pb", pathPrefix, i/shardSize, shards)
		}
		nodeJsons = append(nodeJsons, node.GetJsonString())

	}
	if len(nodeJsons) > 0 {
		err := writeNodeJsonsToFile(nodeJsons, filename)
		if err != nil {
			return nil, nil, err
		}
		jsonFileNames = append(jsonFileNames, filename)
		nodeJsons = nodeJsons[:0]
	}

	linksShardSize := 100000
	linkShards := edges / linksShardSize

	links := make([]*proto.Link, 0, linksShardSize)
	adjListFileName := fmt.Sprintf("%sadjacency_lists_%06d_of_%06d.pb", pathPrefix, 0, linkShards)
	linksFileNames := make([]string, 0, linkShards)
	for i, node := range nodes {

		//fmt.Printf("Processing node %d of %+v\n", i, node)
		if len(node.Outbound) == 0 {
			links = append(links, &proto.Link{
				Src:    int64(i),
				Dest:   int64(i),
				Name:   "end",
				Weight: 1.0,
			})
		}
		numLinks := len(node.Outbound)
		for _, outboundLink := range node.Outbound {
			links = append(links, &proto.Link{
				Src:    int64(i),
				Dest:   int64(indexMap[outboundLink.Node]),
				Name:   outboundLink.Name,
				Labels: outboundLink.Labels,
				Weight: 1.0 / float64(numLinks),
			})
			if len(links) >= linksShardSize {
				err := writeProtoMsgToFile(&proto.Links{TotalNodes: int64(n), Links: links}, adjListFileName)
				if err != nil {
					return nil, nil, err
				}
				linksFileNames = append(linksFileNames, adjListFileName)
				links = links[:0]
				adjListFileName = fmt.Sprintf("%sadjacency_lists_%06d_of_%06d.pb", pathPrefix, len(linksFileNames), linkShards)
			}
		}
	}
	if len(links) > 0 {
		err := writeProtoMsgToFile(&proto.Links{TotalNodes: int64(n), Links: links}, adjListFileName)
		if err != nil {
			return nil, nil, err
		}
		linksFileNames = append(linksFileNames, adjListFileName)
		links = links[:0]
	}

	return jsonFileNames, linksFileNames, nil
}

func writeNodeJsonsToFile(nodeJsons []string, filename string) error {
	// Serialize the message to binary format
	return writeProtoMsgToFile(&proto.Nodes{Json: nodeJsons}, filename)
}

func writeProtoMsgToFile(message proto2.Message, filename string) error {
	data, err := proto2.Marshal(message)
	if err != nil {
		log.Fatalf("Failed to serialize message: %v", err)
		return err
	}
	err = writeFile(filename, data)
	if err != nil {
		return err
	}
	return nil
}

func writeFile(filename string, data []byte) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Write(data)
	if err != nil {
		return err
	}

	return nil
}

func RemoveMergeNodes(root *Node) {
	removed := true
	for removed {
		// The implementation of removeMergeNodes is buggy. It does not remove all the merge nodes.
		// Especially when there are multiple merge nodes pointing to the same node.
		// Temporary hack to calll this multiple times until no more is left
		removed = removeMergeNodes(root, nil, make(map[*Node]bool))
	}
}

func removeMergeNodes(currentNode *Node, parentNode *Node, visited map[*Node]bool) bool {

	if currentNode == nil {
		return false
	}
	if visited[currentNode] {
		return false
	}
	removed := false
	visited[currentNode] = true
	for _, child := range currentNode.Outbound {
		if child.Node.Process == nil && len(child.Node.Outbound) == 1 {
			for j, p := range parentNode.Outbound {
				if p.Node == currentNode {
					parentNode.Outbound[j].Node = child.Node.Outbound[0].Node
				}
			}
			child.Node.Outbound[0].Node.Inbound = append(child.Node.Outbound[0].Node.Inbound, &Link{Node: parentNode})
			//if parentNode == nil || len(parentNode.Outbound) != 1 {
			//	fmt.Printf("parentNode: %p, %s\n", parentNode, parentNode.String())
			//	fmt.Printf("currentNode: %p, %s\n", currentNode, currentNode.String())
			//	fmt.Printf("childNode: %p, %s\n", child, child.String())
			//	panic(fmt.Sprintf("Expecting only one Outbound node for the parent node %p, %s", parentNode, parentNode.String()))
			//}

			//child = child.Outbound[0]
			removed = true
			removeMergeNodes(child.Node.Outbound[0].Node, parentNode, visited)
			continue
		} else if child.Node.Process == nil {
			panic(fmt.Sprintf("Expecting only one Outbound node for the parent node %p, %s", parentNode, parentNode.String()))
		} else {
			removed = removed || removeMergeNodes(child.Node, currentNode, visited)
		}
		//removeMergeNodes(child, currentNode, visited)
	}
	return removed
}

func GenerateDotFile(node *Node, visited map[*Node]bool) string {
	re := regexp.MustCompile(`\\+`)
	dotGraph := "digraph G {\n"

	var dfs func(n *Node)
	dfs = func(n *Node) {
		if visited[n] {
			return
		}
		visited[n] = true

		if n.Process != nil && !n.Process.Enabled {
			return
		}

		nodeID := fmt.Sprintf("\"%p\"", n)

		color := "black"
		if n.Process.HasFailedInvariants() {
			color = "red"
		}
		if n.Process != nil && n.Process.Witness != nil {
			for _, w := range n.Process.Witness {
				for _, pass := range w {
					if pass && color != "red" {
						color = "green"
						// Ideally this should break from outerloop, for now okay. not sure if go has labelled stmts
						break
					}
				}
			}
		}
		penwidth := 1
		if n.Process != nil && len(n.Threads) == 0 {
			penwidth = 2
		}
		stateString := re.ReplaceAllString(n.String(), "\\")
		dotGraph += fmt.Sprintf("  %s [label=\"%s\", color=\"%s\" penwidth=\"%d\" ];\n", nodeID, stateString, color, penwidth)

		// Recursively visit Outbound nodes
		for _, child := range n.Outbound {
			if child.Node.Process != nil && !child.Node.Process.Enabled {
				continue
			}

			childID := fmt.Sprintf("\"%p\"", child.Node)
			label := child.Name
			if child.Labels != nil && len(child.Labels) > 0 {
				label += "[" + strings.Join(child.Labels, ", ") + "]"

			}
			edgewidth := 1
			edgecolor := "black"
			if child.Fairness != proto.FairnessLevel_FAIRNESS_LEVEL_UNKNOWN &&
				child.Fairness != proto.FairnessLevel_FAIRNESS_LEVEL_UNFAIR {
				edgecolor = "forestgreen"
				if child.Fairness == proto.FairnessLevel_FAIRNESS_LEVEL_STRONG {
					edgewidth = 3
				}
			}
			//if color != "green" {
			dotGraph += fmt.Sprintf("  %s -> %s [label=\"%s\", color=\"%s\" penwidth=\"%d\" ];\n", nodeID, childID, label, edgecolor, edgewidth)
			//}

			dfs(child.Node)
		}
	}

	dfs(node)
	dotGraph += "}\n"

	return dotGraph
}

// Helper function to print the graph
func printGraph(node *Node) {
	if node == nil {
		return
	}

	name := ""
	if node.Process != nil {
		name = node.Process.Name
	}
	fmt.Printf("Node: %p, Process: %p (%s)\n", node, node.Process, name)
	for _, outbound := range node.Outbound {
		fmt.Printf("  -> ")
		printGraph(outbound.Node)
	}
}

func GenerateFailurePath(nodes []*Link, invariant *InvariantPosition) string {
	re := regexp.MustCompile(`\\+`)

	builder := strings.Builder{}
	builder.WriteString("digraph G {\n")

	parentID := ""

	visited := map[*Node]string{}
	for i, link := range nodes {
		node := link.Node
		nodeID := fmt.Sprintf("\"%d\"", i)
		if visited[node] != "" {
			//parentID = visited[node]
			nodeID = visited[node]

		} else {
			visited[node] = nodeID
			color := "black"
			if node.Process.HasFailedInvariants() {
				color = "red"
			} else if invariant != nil && node.Process.Witness != nil && node.Process.Witness[invariant.FileIndex][invariant.InvariantIndex] {
				color = "green"
			}
			penwidth := 1
			if node.Process != nil && len(node.Threads) == 0 {
				penwidth = 2
			}
			stateString := re.ReplaceAllString(node.String(), "\\")
			builder.WriteString(fmt.Sprintf("  %s [label=\"%s\", color=\"%s\" penwidth=\"%d\" ];\n", nodeID, stateString, color, penwidth))

		}

		if parentID != "" {
			label := link.Name
			builder.WriteString(fmt.Sprintf("  %s -> %s [label=\"%s\"];\n", parentID, nodeID, label))
		}
		parentID = nodeID
	}



	builder.WriteString("}\n")
	return builder.String()
}

func ReverseLink(node *Node, link *Link) *Link {
	// Shallow copy link.
	// change the link.Node to node
	tmp := *link
	tmp.Node = node
	return &tmp
}
