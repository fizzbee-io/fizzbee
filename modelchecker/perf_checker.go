package modelchecker

import proto "fizz/proto"

func genTransitionMatrix(nodes []*Node, model *proto.PerformanceModel) [][]float64 {
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
        totalProb := 0.0
        missingCount := 0
        linkProbabilities := make(map[*Link]float64)
        for _, outboundLink := range node.Outbound {
            if len(outboundLink.Labels) == 0 {
                missingCount++
                continue
            }
            linkProb := 0.0
            for _, label := range outboundLink.Labels {
                linkProb += model.Configs[label].GetProbability()
            }
            totalProb += linkProb
            linkProbabilities[outboundLink] = linkProb
        }
        if totalProb > 1.0 {
            panic("Total probability for a node cannot exceed 1")
        }
        if totalProb == 0 {
            missingCount = len(node.Outbound)
        }
        missingProb := 0.0
        if missingCount > 0 {
            missingProb = (1.0 - totalProb) / float64(missingCount)
        }
        for _, outboundLink := range node.Outbound {
            prob,found := linkProbabilities[outboundLink]
            if found && totalProb > 0 {
                matrix[indexMap[node]][indexMap[outboundLink.Node]] += prob
            } else {
                matrix[indexMap[node]][indexMap[outboundLink.Node]] += missingProb
            }
        }

    }

    return matrix
}

func genCounterMatrices(nodes []*Node, model *proto.PerformanceModel) map[string][][]float64 {
    matrices := make(map[string][][]float64)
    if model == nil {
        return matrices
    }
    for _, config := range model.Configs {
        for name, _ := range config.Counters {
            if matrices[name] == nil {
                matrices[name] = create2DMatrix(len(nodes))
            }
        }
    }

    indexMap := make(map[*Node]int)
    for i, node := range nodes {
        indexMap[node] = i
    }

    for _, node := range nodes {
        if len(node.Outbound) == 0 {
            continue
        }
        for _, outboundLink := range node.Outbound {
            if len(outboundLink.Labels) == 0 {
                continue
            }
            for _, label := range outboundLink.Labels {
                config := model.Configs[label]
                if config == nil {
                    continue
                }
                for name, counter := range config.Counters {
                    matrices[name][indexMap[node]][indexMap[outboundLink.Node]] += counter.GetNumeric()
                }
            }

        }

    }

    return matrices
}

func create2DMatrix(n int) [][]float64 {
    matrix := make([][]float64, n)
    for i := range matrix {
        matrix[i] = make([]float64, n)
    }
    return matrix
}