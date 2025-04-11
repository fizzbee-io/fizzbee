import json
import time
import sys

import performance.files as files
import performance.markov_chain as markov_chain
import proto.performance_model_pb2 as perf
import proto.fizz_ast_pb2 as ast

import performance.fmt as fmt
import argparse

import matplotlib.pyplot as plt


def plot_histogram(histogram):
    labels = list(histogram[0][1].keys())  # Extract labels from the first tuple
    probabilities = [entry[0] for entry in histogram]  # Extract probabilities
    costs = {label: [entry[1][label] for entry in histogram] for label in labels}  # Extract costs for each label

    # Plot each label
    for label in labels:
        plt.plot(probabilities, costs[label], label=label)

    # Add labels and legend
    plt.xlabel('Probability')
    plt.ylabel('Cost/Reward')
    plt.title('Histogram')
    plt.legend()
    plt.grid(True)

    # Show plot
    plt.show()


def plot_cdf(metrics):
    histogram = metrics.histogram
    mean = metrics.mean
    if len(histogram) == 0:
        print("No histogram")
        return
    labels = list(histogram[0][1].keys())  # Extract labels from the first tuple
    probabilities = [entry[0] for entry in histogram]  # Extract probabilities
    costs = {label: [entry[1][label] for entry in histogram] for label in labels}  # Extract costs for each label

    # Plot CDF for each label
    for label in labels:
        plt.figure()  # Create a new figure for each label
        plt.plot(costs[label], probabilities, label=label)

        # Add labels and legend
        plt.xlabel('Cost/Reward')
        plt.ylabel('Probability')
        plt.title(f'{label} CDF')
        plt.legend()
        plt.grid(True)
        plt.axvline(x=mean[label], color='r', linestyle='--', label=f'Mean = {mean[label]}')


    # Show plots
    plt.show()


def main(argv):
    parser = argparse.ArgumentParser(description='Example of command-line flags in Python')
    parser.add_argument('-s', '--states', type=str, help='Path prefix for the states file')
    parser.add_argument('-m', '--perf', type=str, help='Path for the performance model spec file')
    parser.add_argument('-b', '--source', type=str, help='Path for the behaviour model spec file')

    # Boolean flags with explicit enable/disable options
    parser.add_argument('--steady_state', dest='steady_state', action='store_true', help='Enable steady state (default: True)')
    parser.add_argument('--no_steady_state', dest='steady_state', action='store_false', help='Disable steady state')

    parser.add_argument('--include_witness_in_init', dest='include_witness_in_init', action='store_true', help='Include witness in init (default: True)')
    parser.add_argument('--no_include_witness_in_init', dest='include_witness_in_init', action='store_false', help='Disable witness in init')

    # Choice-based argument
    parser.add_argument('--init_nodes', choices=['root', 'init', 'all', 'steady_state'], default='root', help='Specify initialization nodes')

    # List of strings (comma-separated values)
    parser.add_argument('--invariants', type=lambda s: s.split(','), default=None,
                        help='Comma-separated list of invariants (default: None means all, empty list means actually empty)')

    parser.set_defaults(steady_state=True, include_witness_in_init=True)

    args = parser.parse_args()

    print("States Path:", args.states)
    print("Performance Model Path:", args.perf)
    print("Behavior Model Path:", args.source)
    print("Steady State Enabled:", args.steady_state)
    print("Include Witness in Init:", args.include_witness_in_init)
    print("Init Nodes:", args.init_nodes)
    print("Invariants:", args.invariants)

    if not args.states:
        print("--states (the path prefix for the states data) is required")
        exit(1)

    perf_model = perf.PerformanceModel()
    if args.perf:
        print("perf file", args.perf)
        perf_model = files.load_performance_model_from_file(args.perf)

    source_model = ast.File()
    if args.source:
        print("source file", args.source)
        source_model = files.load_behavior_model_from_file(args.source)
        #print(source_model)
    # print(perf_model)

    nodespb = files.load_nodes_from_proto_files(args.states)
    # print(nodespb)
    nodes = []
    for i, node in enumerate(nodespb.json):
        nodes.append(json.loads(node))

    links = files.load_adj_lists_from_proto_files(args.states)

    trans_matrix = markov_chain.create_transition_matrix_sparse(links, perf_model)
    cost_matrices = markov_chain.create_cost_matrices_sparse(links, perf_model)
    #print('perf_model', perf_model)
    if args.steady_state or args.init_nodes == 'steady_state':
        steady_state,metrics = markov_chain.steady_state_sparse(links, perf_model)
        #print(steady_state)
        print(metrics)

        steady_state_nodes = [(i, prob, nodes[i]) for i, prob in enumerate(steady_state) if prob > 1e-5]
        if len(steady_state_nodes) < 30:
            for i, prob, node in steady_state_nodes:
                print(f'{i:4d}: {prob:.4f} {fmt.get_state_string(node)}')
        else:
            print(f'{len(steady_state_nodes)} states in steady state')

        # plot_histogram(metrics.histogram)
        plot_cdf(metrics)


    for i,invariant in enumerate(source_model.invariants):
        if "always" not in invariant.temporal_operators or  "eventually" not in invariant.temporal_operators:
            continue
        if args.invariants and invariant.name not in args.invariants:
            continue

        init_nodes = []
        if args.init_nodes == 'root':
            init_nodes = [0]
        elif args.init_nodes == 'init':
#             init_nodes = [i for i,node in enumerate(nodes) if node['name'] in ("init", "yield", "crash") and node['stats']['totalActions'] == 0 and (not node["witness"][0][i] or args.include_witness_in_init)]
            init_nodes = [
                k for k, node in enumerate(nodes)
                    if node['name'] in ("init", "yield", "crash")
                        and node['stats']['totalActions'] == 0
                        and (not node["witness"] or not node["witness"][0] or not node["witness"][0][i] or args.include_witness_in_init)
            ]
        elif args.init_nodes == 'all':
            init_nodes = [
                k for k, node in enumerate(nodes)
                    if node['name'] in ("init", "yield", "crash")
                        and (not node["witness"] or not node["witness"][0] or not node["witness"][0][i] or args.include_witness_in_init)
            ]
        elif args.init_nodes == 'steady_state':
            init_nodes = [
                k for k, prob in enumerate(steady_state)
                    if prob > 1e-5
                        and node['name'] in ("init", "yield", "crash")
                        and (not node["witness"] or not node["witness"][0] or not node["witness"][0][i] or args.include_witness_in_init)
            ]

#         for k, node in enumerate(nodes):
#             print(k, node['name'], node['stats'], node)
#             if node['name'] in ("init", "yield") and node['stats']['totalActions'] == 0 and not node["witness"][0][i]:
#                 print("Init node", k, node["witness"][0][i])
#                 init_nodes.append(k)
        print(init_nodes)
        inv_copy = ast.Invariant()
        inv_copy.CopyFrom(invariant)
        inv_copy.ClearField("source_info")
#         print(inv_copy)
        if "eventually" == invariant.temporal_operators[0] and "always" == invariant.temporal_operators[1]:
            print(invariant.name, "eventually always")
            witness_nodes = []
            for j,node in filter(lambda x: x[1]['witness'][0][i], enumerate(nodes)):
                print(j, node)
                witness_nodes.append(j)
            print("Witness nodes", witness_nodes)
            if len(witness_nodes) > 0:
                new_matrix = markov_chain.make_terminal_nodes_sparse(trans_matrix, witness_nodes)
#                 initial_distribution = markov_chain.initial_distribution_from_init_state(links.total_nodes)
                initial_distribution = markov_chain.initial_distribution_from_init_states(links.total_nodes, init_nodes)
                first_stable_states,stabilization_metrics = markov_chain.analyze_sparse(new_matrix, cost_matrices,initial_distribution)
                print(first_stable_states)
                print(stabilization_metrics)

        elif "always" == invariant.temporal_operators[0] and "eventually" == invariant.temporal_operators[1]:
            print(invariant.name, "always eventually")
            witness_nodes = []
            for j,node in filter(lambda x: x[1]['witness'][0][i], enumerate(nodes)):
                print(j, node)
                witness_nodes.append(j)

            live_prob,metrics = markov_chain.steady_state_liveness(links, perf_model, witness_nodes)
            print(live_prob)
            print(metrics)
            plot_cdf(metrics)
            dead_nodes = []
            for j,prob in enumerate(live_prob):
                if prob > 1e-4:
                    state = "LIVE"
                    if not nodes[j]['witness'][0][i]:
                        dead_nodes.append((j, prob, nodes[j]))
                        state = "DEAD"

                    print(f'{state} {j:4d}: {prob:.4f} {fmt.get_state_string(nodes[j])}')
            # for j,prob,node in steady_state_nodes:
            #     if node['witness'][0][i]:
            #         print("LIVE", i,j,prob,node)
            #     else:
            #         print("DEAD", i,j,prob,node)
            # print(witness_nodes)
            # print(trans_matrix)
            # new_matrix = markov_chain.make_terminal_nodes(trans_matrix, witness_nodes)
            #
            # print(new_matrix)
            # _,metrics = markov_chain.steady_state(links, perf_model, new_matrix)


    # markov_chain.create_cost_matrices(links, perf_model)
    # Time to reach steady state
    # Clone the transition matrix, and for each


if __name__ == '__main__':
    main(sys.argv)
