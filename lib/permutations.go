package lib

// permute generates all permutations of the array and appends them to the result.
func permute[T any](arr []T, l int, r int, result *[][]T) {
    if l == r {
        // Append a copy of the current permutation to the result.
        perm := make([]T, len(arr))
        copy(perm, arr)
        *result = append(*result, perm)
    } else {
        for i := l; i <= r; i++ {
            // Swap elements at positions l and i.
            arr[l], arr[i] = arr[i], arr[l]
            // Generate permutations for the rest of the array.
            permute(arr, l+1, r, result)
            // Backtrack by swapping the elements back.
            arr[l], arr[i] = arr[i], arr[l]
        }
    }
}

// GeneratePermutations is a helper function that initializes the result and calls the permute function.
func GeneratePermutations[T any](arr []T) [][]T {
    var result [][]T
    permute(arr, 0, len(arr)-1, &result)
    return result
}

// combinePermutations recursively generates all combinations of the permutations of each subarray.
func combinePermutations[T any](arrays [][][]T, index int, current [][]T, result *[][][]T) {
    if index == len(arrays) {
        // Append a copy of the current combination to the result.
        combination := make([][]T, len(current))
        for i, arr := range current {
            combination[i] = make([]T, len(arr))
            copy(combination[i], arr)
        }
        *result = append(*result, combination)
        return
    }

    // Iterate through all permutations of the current subarray.
    for _, perm := range arrays[index] {
        // Add the current permutation to the current combination.
        current = append(current, perm)
        // Recursively generate combinations for the next subarrays.
        combinePermutations(arrays, index+1, current, result)
        // Remove the current permutation from the current combination.
        current = current[:len(current)-1]
    }
}

// GenerateAllPermutations generates all permutations of each subarray and all combinations of these permutations.
func GenerateAllPermutations[T any](arr [][]T) [][][]T {
    // Generate permutations for each subarray.
    var allPermutations [][][]T
    for _, subarray := range arr {
        permutations := GeneratePermutations(subarray)
        allPermutations = append(allPermutations, permutations)
    }

    // Generate all combinations of the permutations.
    var result [][][]T
    combinePermutations(allPermutations, 0, [][]T{}, &result)
    return result
}

