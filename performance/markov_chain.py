import numpy as np
import scipy.sparse as sp


class Metrics:
    def __init__(self):
        self.mean = {}
        self.histogram = []

    def add_histogram(self, percentile, counters):
        new_counters = {}
        for counter in counters:
            new_counters[counter] = counters[counter]
        self.histogram.append((percentile, new_counters))

    def __str__(self):
        return f"Metrics(mean={self.mean}, histogram={self.histogram})"


# def update_transition_matrix(matrix, links):
#     for link in links.links:
#         matrix[link.src][link.dest] += link.weight


def update_transition_matrix(matrix, links, model):
    total_prob = np.zeros(links.total_nodes, dtype=np.double)
    missing_prob = np.zeros(links.total_nodes, dtype=np.double)
    missing_count = np.zeros(links.total_nodes, dtype=np.int32)
    out_degree = np.zeros(links.total_nodes, dtype=np.int32)
    link_probs = {}

    for i,link in enumerate(links.links):
        # print(i, link)
        out_degree[link.src] = round(1 / link.weight)

        if len(link.labels) == 0:
            missing_count[link.src] += 1
            continue

        link_prob = 0.0
        for label in link.labels:
            # print(label, model.configs[label])
            link_prob += model.configs[label].probability

        total_prob[link.src] += link_prob
        # matrix[link.src][link.dest] += link_prob
        link_probs[i] = link_prob

    for i in range(links.total_nodes):
        if total_prob[i] == 0:
            missing_count[i] = out_degree[i]
        if missing_count[i] > 0:
            # missingProb = (1.0 - totalProb) / float64(missingCount)
            missing_prob[i] = (1.0 - total_prob[i]) / missing_count[i]

    for i,link in enumerate(links.links):
        if i in link_probs and total_prob[link.src] > 0:
            matrix[link.src][link.dest] += link_probs[i]
        else:
            matrix[link.src][link.dest] += missing_prob[link.src]

    print('total_prob\n', total_prob)
    print('missing_prob\n', missing_prob)
    print('missing_count\n', missing_count)
    print('out_degree\n', out_degree)
    # print('link_probs\n', link_probs)


def create_transition_matrix(links, model):
    matrix = np.zeros((links.total_nodes, links.total_nodes), dtype=np.double)
    update_transition_matrix(matrix, links, model)
    return matrix


def create_transition_matrix_sparse(links, model):
    # matrix = np.zeros((links.total_nodes, links.total_nodes), dtype=np.double)
    matrix = sp.dok_array((links.total_nodes, links.total_nodes), dtype=float)

    # update_transition_matrix(matrix, links, model)

    total_prob = np.zeros(links.total_nodes, dtype=np.double)
    missing_prob = np.zeros(links.total_nodes, dtype=np.double)
    missing_count = np.zeros(links.total_nodes, dtype=np.int32)
    out_degree = np.zeros(links.total_nodes, dtype=np.int32)
    link_probs = {}

    for i,link in enumerate(links.links):
        # print(i, link)
        out_degree[link.src] = round(1 / link.weight)

        if len(link.labels) == 0:
            missing_count[link.src] += 1
            continue

        link_prob = 0.0
        for label in link.labels:
            # print(label, model.configs[label])
            link_prob += model.configs[label].probability

        total_prob[link.src] += link_prob
        # matrix[link.src][link.dest] += link_prob
        link_probs[i] = link_prob

    for i in range(links.total_nodes):
        if total_prob[i] == 0:
            missing_count[i] = out_degree[i]
        if missing_count[i] > 0:
            # missingProb = (1.0 - totalProb) / float64(missingCount)
            missing_prob[i] = (1.0 - total_prob[i]) / missing_count[i]

    for i,link in enumerate(links.links):
        if i in link_probs and total_prob[link.src] > 0:
            matrix[link.src,link.dest] += link_probs[i]
        else:
            matrix[link.src,link.dest] += missing_prob[link.src]

    print('total_prob\n', total_prob)
    print('missing_prob\n', missing_prob)
    print('missing_count\n', missing_count)
    print('out_degree\n', out_degree)

    return matrix.tocsr()


def create_cost_matrices_sparse(links, model):
    if not model:
        return {}

    cost_matrices = {}
    # print('model', model)
    for label in model.configs:
        # print(label, model.configs[label])
        for counter in model.configs[label].counters:
            if counter not in cost_matrices:
                # cost_matrices[counter] = np.zeros((links.total_nodes, links.total_nodes), dtype=np.double)
                cost_matrices[counter] = sp.dok_array((links.total_nodes, links.total_nodes), dtype=np.double)

    # for each link, iterate over each label, and add the cost to the cost matrix
    for link in links.links:
        for label in link.labels:
            if label not in model.configs:
                continue
            config = model.configs[label]
            for counter in config.counters:
                # print(counter, link.src, link.dest, config.counters[counter])
                cost_matrices[counter][link.src,link.dest] += config.counters[counter].numeric

    print('cost_matrices', cost_matrices)
    csr_matrices = {}
    for counter in cost_matrices:
        csr_matrices[counter] = cost_matrices[counter].tocsr()
    return csr_matrices


def create_cost_matrices(links, model):
    if not model:
        return {}

    cost_matrices = {}
    # print('model', model)
    for label in model.configs:
        # print(label, model.configs[label])
        for counter in model.configs[label].counters:
            if counter not in cost_matrices:
                cost_matrices[counter] = np.zeros((links.total_nodes, links.total_nodes), dtype=np.double)

    # for each link, iterate over each label, and add the cost to the cost matrix
    for link in links.links:
        for label in link.labels:
            if label not in model.configs:
                continue
            config = model.configs[label]
            for counter in config.counters:
                # print(counter, link.src, link.dest, config.counters[counter])
                cost_matrices[counter][link.src][link.dest] += config.counters[counter].numeric

    print('cost_matrices', cost_matrices)
    return cost_matrices


def analyze(matrix, cost_matrices, initial_distribution, num_iterations=2000, tolerance=1e-12):
    """
    Runs the power iteration algorithm to analyze the markov chain. Specifically, it does two things:
    1. It computes the steady state distribution of the markov chain.
    2. It computes the metrics (mean/histogram) of each counter in the performance model.

    Steady state distribution is computed as next_dist = current_dist * transition_matrix.
    Expected cost of each transition = cost of each transition * probability of the transition
    expected_cost_to_stead_state = current_dist * (expected_transition_cost) for each label.

    To compute the histogram, the logic is a bit complicated. In each iteration, for each absorbing state,
    set the current_dist to 0 and normalize the rest of the states' probability to 1, and continue the iteration.

    :param matrix:
    :param cost_matrices:
    :param initial_distribution:
    :param num_iterations:
    :param tolerance:
    :return:
    """
    n = len(matrix)
    dist = initial_distribution.copy()
    alt_dist = initial_distribution.copy()

    expected_cost_matrices = {}
    mean_counters = {}
    raw_counters = {}
    metrics = Metrics()
    for counter in cost_matrices:
        expected_cost_matrices[counter] = cost_matrices[counter] * matrix
        mean_counters[counter] = 0.0
        raw_counters[counter] = 0.0

    prev_termination_prob = 0.0
    change = 1.0
    for i in range(num_iterations):
        termination_prob = 0.0
        for counter in cost_matrices:
            mean_counters[counter] += sum(np.dot(dist, expected_cost_matrices[counter]))
            raw_counters[counter] += sum(np.dot(alt_dist, expected_cost_matrices[counter]))

        new_dist = np.dot(dist, matrix)
        alt_dist = np.dot(alt_dist, matrix)

        for j in range(n):
            if matrix[j][j] == 1:
                # print(new_dist[j])
                termination_prob += new_dist[j]
                alt_dist[j] = 0.0
        total_prob = sum(alt_dist)
        for j in range(n):
            alt_dist[j] = alt_dist[j] / total_prob
            # mean_counters[counter] += sum(cost)

        # print(i, dist)
        # print(i, new_dist)
        # print(i, alt_dist)
        # print(i, mean_counters)

        if termination_prob > prev_termination_prob:
            metrics.add_histogram(termination_prob, raw_counters)

        prev_termination_prob = termination_prob

        change = np.linalg.norm(new_dist - dist)
        dist = new_dist
        if change < tolerance:
            print(f"Convergence reached after {i+1} iterations.")
            break

    if change >= tolerance:
        print(f"Convergence not reached after {num_iterations} iterations.")

    metrics.mean = mean_counters
    return dist,metrics


def analyze_sparse(matrix, cost_matrices, initial_distribution, num_iterations=2000, tolerance=1e-12):
    """
    Runs the power iteration algorithm to analyze the markov chain. Specifically, it does two things:
    1. It computes the steady state distribution of the markov chain.
    2. It computes the metrics (mean/histogram) of each counter in the performance model.

    Steady state distribution is computed as next_dist = current_dist * transition_matrix.
    Expected cost of each transition = cost of each transition * probability of the transition
    expected_cost_to_stead_state = current_dist * (expected_transition_cost) for each label.

    To compute the histogram, the logic is a bit complicated. In each iteration, for each absorbing state,
    set the current_dist to 0 and normalize the rest of the states' probability to 1, and continue the iteration.

    :param matrix:
    :param cost_matrices:
    :param initial_distribution:
    :param num_iterations:
    :param tolerance:
    :return:
    """
    n = matrix.shape[0]
    dist = sp.coo_array(initial_distribution)
    alt_dist = sp.coo_array(initial_distribution)
    # print("matrix\n", matrix)
    matrix = sp.csr_matrix(matrix)
    # print(matrix)
    # print("matrix.getformat()", matrix.getformat())
    # print("dist.getformat()", dist.getformat())
    # dist = initial_distribution.copy()
    # alt_dist = initial_distribution.copy()

    expected_cost_matrices = {}
    mean_counters = {}
    raw_counters = {}
    metrics = Metrics()
    for counter in cost_matrices:
        print("counter", counter)
        print(cost_matrices[counter])

        expected_cost_matrices[counter] = sp.csr_matrix(cost_matrices[counter]).multiply(matrix)
        # expected_cost_matrices[counter] = cost_matrices[counter] * matrix.toarray()
        # sp.csr_matrix(cost_matrices[counter]) * matrix
        # print(expected_cost_matrices[counter].getformat())
        # print(expected_cost_matrices[counter])
        # print(expected_cost_matrices[counter].toarray())

        mean_counters[counter] = 0.0
        raw_counters[counter] = 0.0
    # print("expected_cost_matrices", expected_cost_matrices)
    prev_termination_prob = 0.0
    change = 1.0
    reset_tolerance = tolerance / n
    for i in range(num_iterations):
        termination_prob = 0.0
        for counter in cost_matrices:
            mean_counters[counter] += np.sum(dist.dot(expected_cost_matrices[counter]))
            raw_counters[counter] += np.sum(alt_dist.dot(expected_cost_matrices[counter]))

        new_dist = dist.dot(matrix)
        alt_dist = alt_dist.dot(matrix)
        if i == 0:
            print("new_dist.getformat()", new_dist.getformat())
        # print("new_dist", new_dist)
        # print("new_dist.shape", alt_dist.shape)
        # new_dist = np.dot(dist, matrix)
        # alt_dist = np.dot(alt_dist, matrix)

        for j in range(n):
            if matrix[j,j] == 1:
                # print(new_dist[j])
                termination_prob += new_dist[0,j]
                alt_dist[0,j] = 0.0

        total_prob = np.sum(alt_dist)
        # print("alt_dist", alt_dist.__class__, "total_prob", total_prob.__class__, total_prob)
        alt_dist = alt_dist / total_prob
        # for j in range(n):
        #     alt_dist[j] = alt_dist[j] / total_prob

        new_dist.data[new_dist.data < reset_tolerance] = 0
        non_zero_sum = np.sum(new_dist.data)
        new_dist.data /= non_zero_sum

        # if i % 100 == 0:
        #     # print(i, dist)
        #     print(i, new_dist.nnz, new_dist)
        #     # print(i, alt_dist)
        #     # print(i, mean_counters)

        if termination_prob > prev_termination_prob:
            metrics.add_histogram(termination_prob, raw_counters)

        prev_termination_prob = termination_prob

        change = sp.linalg.norm(new_dist - dist)
        dist = new_dist
        if change < tolerance:
            print(f"Convergence reached after {i+1} iterations.")
            break

    if change >= tolerance:
        print(f"Convergence not reached after {num_iterations} iterations.")

    metrics.mean = mean_counters
    return dist.toarray()[0], metrics


def initial_distribution_from_init_state(n):
    # Create a vector of length n with all elements = 0 except the first element = 1
    v = np.zeros(n)
    v[0] = 1
    return v


def initial_distribution_from_any_states(n):
    # Create a vector of length n with all elements = 0 except the first element = 1
    v = np.full((n), 1/n)
    return v


def steady_state(links, perf_model):
    matrix = create_transition_matrix(links, perf_model)
    cost_matrices = create_cost_matrices(links, perf_model)

    initial_distribution = initial_distribution_from_init_state(links.total_nodes)
    if links.total_nodes < 30:
        print(matrix)
        print(initial_distribution)
    prob,metrics = analyze(matrix, cost_matrices, initial_distribution)
    return prob,metrics


def steady_state_sparse(links, perf_model,matrix=None, cost_matrices=None):
    if matrix is None:
        matrix = create_transition_matrix_sparse(links, perf_model)

    if cost_matrices is None:
        cost_matrices = create_cost_matrices_sparse(links, perf_model)

    initial_distribution = initial_distribution_from_init_state(links.total_nodes)
    if links.total_nodes < 30:
        print(matrix)
        print(initial_distribution)
    prob,metrics = analyze_sparse(matrix, cost_matrices, initial_distribution)
    return prob,metrics


def make_terminal_nodes(transition_matrix, terminal_nodes):
    # Create a copy of the transition matrix to avoid modifying the original
    modified_matrix = np.copy(transition_matrix)

    # Iterate over each terminal node
    for node in terminal_nodes:
        # Set all out probability to 0 and self-loop probability to 1 for the terminal node
        modified_matrix[node, :] = 0
        modified_matrix[node, node] = 1

    return modified_matrix


def make_terminal_nodes_sparse(transition_matrix, terminal_nodes):
    # Create a copy of the CSR matrix to avoid modifying the original
    modified_matrix = transition_matrix.copy()

    # Iterate over each terminal node
    for node in terminal_nodes:
        # Set all out probability to 0 and self-loop probability to 1 for the terminal node
        modified_matrix[node, :] = sp.csr_matrix((1, transition_matrix.shape[1]), dtype=transition_matrix.dtype)
        modified_matrix[node, node] = 1

    return modified_matrix


def steady_state_liveness(links, perf_model, terminal_nodes):
    trans_matrix = create_transition_matrix_sparse(links, perf_model)
    # print(trans_matrix)
    matrix = make_terminal_nodes_sparse(trans_matrix, terminal_nodes)
    cost_matrices = create_cost_matrices_sparse(links, perf_model)
    # print(matrix)
    initial_distribution = initial_distribution_from_any_states(links.total_nodes)
    # print(initial_distribution)
    prob,metrics = analyze_sparse(matrix, cost_matrices, initial_distribution)
    return prob,metrics


