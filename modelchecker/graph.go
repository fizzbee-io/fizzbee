package modelchecker

import (
	"encoding/json"
	"fizz/proto"
	"fmt"
	"github.com/fizzbee-io/fizzbee/lib"
	proto3 "google.golang.org/protobuf/proto"
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

	shardSize := 1000000
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

	linksShardSize := 10000000
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
				Type:   "action",
			})
		}
		numLinks := len(node.Outbound)
		for _, outboundLink := range node.Outbound {
			intMap := make(map[int64]int64)
			for k, v := range outboundLink.ThreadsMap {
				intMap[int64(k)] = int64(v)
			}
			outLinkName := outboundLink.Name
			outLinkName = strings.ReplaceAll(outLinkName, lib.SymmetryPrefix, "")
			links = append(links, &proto.Link{
				ReqId:           int64(outboundLink.ReqId),
				Src:             int64(i),
				Dest:            int64(indexMap[outboundLink.Node]),
				Name:            outLinkName,
				Labels:          outboundLink.Labels,
				Messages:        outboundLink.Messages,
				Weight:          1.0 / float64(numLinks),
				NewToOldThreads: intMap,
				Type:            outboundLink.Type,
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

func GenerateErrorPathProtoOfJson(errorPath []*Link, pathPrefix string) ([]string, []string, error) {
	dir := filepath.Dir(pathPrefix)
	err := os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		fmt.Printf("Error creating directory %s: %v\n", dir, err)
		return nil, nil, err
	}

	indexMap := make(map[*Node]int)
	filename := fmt.Sprintf("%snodes_errors.pb", pathPrefix)
	nodeJsons := make([]string, 0, len(errorPath))
	jsonFileNames := make([]string, 0, 1)
	edges := len(errorPath) - 1
	for i, errorPathLink := range errorPath {
		node := errorPathLink.Node
		indexMap[node] = i
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

	links := make([]*proto.Link, 0, edges)
	adjListFileName := fmt.Sprintf("%sadjacency_lists_errors.pb", pathPrefix)
	linksFileNames := make([]string, 0, 1)
	for i, outboundLink := range errorPath[1:] {
		outLinkName := outboundLink.Name
		outLinkName = strings.ReplaceAll(outLinkName, lib.SymmetryPrefix, "")
		protoLink := &proto.Link{
			ReqId:    int64(outboundLink.ReqId),
			Src:      int64(i),
			Dest:     int64(i + 1),
			Name:     outboundLink.Name,
			Labels:   outboundLink.Labels,
			Messages: outboundLink.Messages,
			Weight:   1.0,
		}
		links = append(links, protoLink)

	}
	if len(links) > 0 {
		err := writeProtoMsgToFile(&proto.Links{TotalNodes: int64(len(errorPath)), Links: links}, adjListFileName)
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

func writeProtoMsgToFile(message proto3.Message, filename string) error {
	data, err := proto3.Marshal(message)
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
		if n.Process != nil && (n.GetThreadsCount() == 0 || n.Name == "yield") {
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
			if len(child.ThreadsMap) > 0 {
				label += fmt.Sprintf("\n ThreadsMap: %v", child.ThreadsMap)
			}
			label = strings.ReplaceAll(label, "\"", "\\\"")
			edgewidth := 1
			edgecolor := "black"

			if child.Fairness != proto.FairnessLevel_FAIRNESS_LEVEL_UNKNOWN &&
				child.Fairness != proto.FairnessLevel_FAIRNESS_LEVEL_UNFAIR {
				edgecolor = "forestgreen"
				if child.Fairness == proto.FairnessLevel_FAIRNESS_LEVEL_STRONG {
					edgewidth = 3
				}
			}
			if child.HasFailedInvariants() {
				edgecolor = "red"
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
			if node.Process != nil && node.GetThreadsCount() == 0 || node.Name == "yield" {
				penwidth = 2
			}
			stateString := re.ReplaceAllString(node.String(), "\\")
			builder.WriteString(fmt.Sprintf("  %s [label=\"%s\", color=\"%s\" penwidth=\"%d\" ];\n", nodeID, stateString, color, penwidth))

		}

		if parentID != "" {
			label := link.Name
			label = strings.ReplaceAll(label, "\"", "\\\"")
			edgewidth := 1
			edgecolor := "black"

			if link.Fairness != proto.FairnessLevel_FAIRNESS_LEVEL_UNKNOWN &&
				link.Fairness != proto.FairnessLevel_FAIRNESS_LEVEL_UNFAIR {
				edgecolor = "forestgreen"
				if link.Fairness == proto.FairnessLevel_FAIRNESS_LEVEL_STRONG {
					edgewidth = 3
				}
			}
			if link.HasFailedInvariants() {
				edgecolor = "red"
			}
			builder.WriteString(fmt.Sprintf("  %s -> %s [label=\"%s\", color=\"%s\" penwidth=\"%d\"];\n", parentID, nodeID, fmt.Sprintf("%d: %s", i, label), edgecolor, edgewidth))
		}
		parentID = nodeID
	}

	builder.WriteString("}\n")
	return builder.String()
}

type Pair[T1 any, T2 any] struct {
	First  T1
	Second T2
}

func GenerateCommunicationGraph(messages []string) string {
	// TODO: Handle assymetric communication
	// For now, if there are 2 senders and 3 receivers, we will have 6 possible communication links
	// We see of sender[0] sends to receiver[0], we assume, all 6 possible links are possible
	// This is not perfect, but should be good enough for now
	// Also, for now we are assuming 2 or more roles as any number of roles can be there
	builder := strings.Builder{}
	builder.WriteString("digraph G {\n")
	builder.WriteString("  node [shape=box];\n")
	builder.WriteString("  splines=false;\n")

	builder.WriteString("  rankdir=LR;\n")

	roles := make(map[string]bool)
	// tracks whether there are a single instance or a role or multiple instances
	roleNames := make(map[string]bool)
	// first key is [sender, receiver] pair, second key is message name
	uniqueMessages := make(map[Pair[string, string]]map[string]bool)
	// first key is [receiver], second key is action name, fairness level pair
	uniqueActions := make(map[string]map[string]proto.FairnessLevel)

	for _, message := range messages {
		dict := make(map[string]interface{})
		_ = json.Unmarshal([]byte(message), &dict)
		if dict["type"] == "message" && dict["sender"] != "" {
			sender := dict["sender"].(string)
			senderParts := strings.Split(sender, "#")
			roles[sender] = true
			roleNames[senderParts[0]] = roleNames[senderParts[0]] || senderParts[1] != "0"

			receiver := dict["receiver"].(string)
			receiverParts := strings.Split(receiver, "#")
			roles[receiver] = true
			roleNames[receiverParts[0]] = roleNames[receiverParts[0]] || receiverParts[1] != "0"

			pair := Pair[string, string]{senderParts[0], receiverParts[0]}
			if _, ok := uniqueMessages[pair]; !ok {
				uniqueMessages[pair] = make(map[string]bool)
			}
			uniqueMessages[pair][dict["name"].(string)] = true
		} else if dict["type"] == "action" || dict["sender"] == "" {
			receiver := dict["receiver"].(string)
			receiverParts := strings.Split(receiver, "#")
			roles[receiver] = true
			roleNames[receiverParts[0]] = roleNames[receiverParts[0]] || receiverParts[1] != "0"

			if _, ok := uniqueActions[receiverParts[0]]; !ok {
				uniqueActions[receiverParts[0]] = make(map[string]proto.FairnessLevel)
			}
			if dict["fairness"] == nil {
				uniqueActions[receiverParts[0]][dict["name"].(string)] = proto.FairnessLevel(proto.FairnessLevel_FAIRNESS_LEVEL_UNKNOWN)
			} else {
				uniqueActions[receiverParts[0]][dict["name"].(string)] = proto.FairnessLevel(int(dict["fairness"].(float64)))
			}
		}
	}

	for roleName, replicated := range roleNames {
		if replicated {
			// Add a html type node table to represent replicated service
			// for example, if there are multiple instances of Participant, we will have
			//     "Participant" [shape=none label=<
			//      <table cellpadding="14" cellspacing="8" style="dashed">
			//      <tr><td port="p0">Participant #0</td></tr>
			//      <tr><td port="p1" border="0">&#x022EE;</td></tr>
			//      <tr><td port="p2">Participant #2</td></tr>
			//      </table>>]
			builder.WriteString(fmt.Sprintf("  \"%s\" [shape=none label=<<table cellpadding=\"14\" cellspacing=\"8\" style=\"dashed\">\n", roleName))
			builder.WriteString(fmt.Sprintf("      <tr><td port=\"p%d\">%s#%d</td></tr>\n", 0, roleName, 0))
			builder.WriteString(fmt.Sprintf("      <tr><td port=\"p%d\" border=\"0\">&#x022EE;</td></tr>\n", 1))
			builder.WriteString(fmt.Sprintf("      <tr><td port=\"p%d\">%s#%d</td></tr>\n", 2, roleName, 2))
			builder.WriteString("      </table>>]\n")
		}
	}
	for p, m := range uniqueMessages {
		sender := p.First
		receiver := p.Second
		// Concatenate all the messages
		label := ""
		for message := range m {
			if label != "" {
				label += ", "
			}
			label += message
		}
		// add a edge between sender and receiver
		senderPorts := []string{}
		if roleNames[sender] {
			senderPorts = []string{":p0", ":p2"}
		} else {
			senderPorts = []string{""}

		}
		receiverPorts := []string{}
		if roleNames[receiver] {
			receiverPorts = []string{":p0", ":p2"}
		} else {
			receiverPorts = []string{""}
		}
		for _, senderPort := range senderPorts {
			for _, receiverPort := range receiverPorts {
				builder.WriteString(fmt.Sprintf("  \"%s\"%s -> \"%s\"%s [label=\"%s\"];\n", sender, senderPort, receiver, receiverPort, label))
			}
		}
	}

	for receiver, actions := range uniqueActions {
		// for each receiver, add the actions
		// - if the action is unfair add a hidden node and add an edge from the action to the receiver with action name as label
		// - if there is at least one fair action, add a html table hidden node with one row for each action with no label and port name as action name
		// - add an edge from corresponding action to the receiver with action name as label

		// Add a hidden node for each action
		hasFairAction := false
		for action, fairness := range actions {

			if fairness == proto.FairnessLevel_FAIRNESS_LEVEL_UNKNOWN || fairness == proto.FairnessLevel_FAIRNESS_LEVEL_UNFAIR {
				actionNodeName := fmt.Sprintf("action%s%s", receiver, action)
				builder.WriteString(fmt.Sprintf("  \"%s\" [label=\"\" shape=\"none\"]\n", actionNodeName))
				builder.WriteString(fmt.Sprintf("  \"%s\" -> \"%s\" [label=\"%s\"];\n", actionNodeName, receiver, action))
			} else {
				hasFairAction = true
			}
		}
		if hasFairAction {
			builder.WriteString(fmt.Sprintf("  \"FairAction%s\" [shape=none label=<<table cellpadding=\"14\" cellspacing=\"8\" style=\"invisible\"><tr>\n", receiver))
			for action, fairness := range actions {
				if !(fairness == proto.FairnessLevel_FAIRNESS_LEVEL_UNKNOWN || fairness == proto.FairnessLevel_FAIRNESS_LEVEL_UNFAIR) {
					builder.WriteString(fmt.Sprintf("      <td port=\"%s\"></td>\n", action))
				}
			}
			builder.WriteString("      </tr></table>>]\n")
			// Make the FairAction node at the same rank as the receiver
			builder.WriteString(fmt.Sprintf("  { rank=same; \"%s\"; \"FairAction%s\"; }\n", receiver, receiver))
			for action, fairness := range actions {
				if !(fairness == proto.FairnessLevel_FAIRNESS_LEVEL_UNKNOWN || fairness == proto.FairnessLevel_FAIRNESS_LEVEL_UNFAIR) {
					builder.WriteString(fmt.Sprintf("  \"FairAction%s\":%s -> \"%s\" [label=\"%s\"];\n", receiver, action, receiver, action))
				}
			}
		}
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
