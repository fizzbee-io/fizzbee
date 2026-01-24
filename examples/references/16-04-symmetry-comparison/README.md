# Symmetry Reduction Comparison

This example demonstrates the impact of symmetry reduction by providing two nearly-identical specs:

## Files

- **WithoutSymmetry.fizz**: Regular roles with list
- **WithSymmetry.fizz**: Symmetric roles with bag

## How to Compare

```bash
# Run without symmetry
fizz WithoutSymmetry.fizz
# Note the output: X unique states, Y nodes

# Run with symmetry
fizz WithSymmetry.fizz
# Note the output: Much fewer states!
```

## Expected Results

With `NUM_PROCESSES = 2` and `max_actions = 6`:

| Version | Approx States | Reduction |
|---------|---------------|-----------|
| Without | ~40-60 states | Baseline |
| With | ~20-30 states | **~50% reduction** |

## Try Scaling Up

Edit both files and increase `NUM_PROCESSES`:

```python
NUM_PROCESSES = 3  # Change to 3 or 4
```

Expected reduction factors:
- 2 processes: 2! = 2x reduction (~50%)
- 3 processes: 3! = 6x reduction (~83%)
- 4 processes: 4! = 24x reduction (~96%)

## Key Differences

| Aspect | WithoutSymmetry.fizz | WithSymmetry.fizz |
|--------|---------------------|-------------------|
| Role declaration | `role Process:` | `symmetric role Process:` |
| Collection | `processes = []` | `processes = bag()` |
| Add to collection | `.append(Process())` | `.add(Process())` |
| State space | Larger | Smaller |

## The Lesson

**Always use `symmetric role` + `bag()`/`set()` for indistinguishable instances!**

This simple change can make the difference between:
- Model checking completes in seconds vs hours
- Being able to verify 3 nodes vs 10 nodes
- Finding bugs vs giving up due to state explosion
