"""Rewrite the embedded ONNX content-type model into ops that run fast on gomlx's Go backend.

The rewrites are mathematically equivalent to the original graph:

1. GlobalMaxPool -> ReduceMax over the spatial axes (onnx-gomlx lacks it).
2. one-hot (Reshape/Expand/Equal/Cast) + MatMul -> Gather of the weight rows.
3. Conv (1D, no padding, stride 1) -> im2col (Slice + Concat) + MatMul, so it
   runs on the backend's optimized matmul kernels instead of its generic conv.

Unused nodes and initializers are then dropped. With --check, the patched
model is compared against the original on random inputs with onnxruntime.

usage: python -I patch_model.py [--check] IN.onnx OUT.onnx
"""

import argparse
import sys

import numpy as np
import onnx
from onnx import helper, numpy_helper, shape_inference

ap = argparse.ArgumentParser()
ap.add_argument("--check", action="store_true")
ap.add_argument("src")
ap.add_argument("dst")
args = ap.parse_args()

m = onnx.load(args.src)
g = m.graph
inits = {t.name: t for t in g.initializer}
inferred = shape_inference.infer_shapes(m)
shapes = {
    vi.name: [d.dim_value if d.HasField("dim_value") else None for d in vi.type.tensor_type.shape.dim]
    for vi in list(inferred.graph.value_info) + list(inferred.graph.input) + list(inferred.graph.output)
}
producer = {o: n for n in g.node for o in n.output}


def init_array(name):
    return numpy_helper.to_array(inits[name]) if name in inits else None


def add_init(name, arr):
    t = numpy_helper.from_array(arr, name)
    g.initializer.append(t)
    inits[name] = t
    return name


def attrs(n):
    return {a.name: helper.get_attribute_value(a) for a in n.attribute}


replace = {}  # node name -> list of replacement nodes


# 1. GlobalMaxPool -> ReduceMax.
for n in g.node:
    if n.op_type != "GlobalMaxPool":
        continue
    rank = len(shapes[n.input[0]])
    replace[n.name] = [helper.make_node("ReduceMax", list(n.input), list(n.output), name=n.name, axes=list(range(2, rank)), keepdims=1)]
    print(f"GlobalMaxPool {n.name}: ReduceMax axes {list(range(2, rank))}")


# 2. one-hot + MatMul -> Gather. Pattern:
#    MatMul(Cast(Equal(Expand(Reshape(idx, [-1, L, 1]), _), iota[1,1,V])), W[V, D])
for n in g.node:
    if n.op_type != "MatMul" or init_array(n.input[1]) is None:
        continue
    cast = producer.get(n.input[0])
    if cast is None or cast.op_type != "Cast":
        continue
    eq = producer.get(cast.input[0])
    if eq is None or eq.op_type != "Equal":
        continue
    expand, iota = producer.get(eq.input[0]), init_array(eq.input[1])
    if expand is None or expand.op_type != "Expand" or iota is None:
        continue
    reshape = producer.get(expand.input[0])
    if reshape is None or reshape.op_type != "Reshape":
        continue
    w = init_array(n.input[1])
    vocab = iota.shape[-1]
    if not (iota.reshape(-1) == np.arange(vocab)).all() or w.shape[0] != vocab:
        sys.exit(f"unexpected one-hot pattern at {n.name}")
    idx = reshape.input[0]
    replace[n.name] = [helper.make_node("Gather", [n.input[1], idx], list(n.output), name=n.name + "/gather", axis=0)]
    print(f"one-hot MatMul {n.name}: Gather from {list(w.shape)} with indices {idx!r}")


# 3. Conv1D -> im2col + MatMul. Pattern (from jax2tf):
#    Squeeze(Slice(Conv(Transpose(Slice(Unsqueeze(x[N, L, C], 3)), perm 0,2,1,3), W[O, C, K, 1])), 3)
for n in g.node:
    if n.op_type != "Conv":
        continue
    a = attrs(n)
    w = init_array(n.input[1])
    if w is None or len(n.input) > 2 or w.ndim != 4 or w.shape[3] != 1:
        sys.exit(f"unsupported Conv {n.name}")
    if a.get("strides", [1, 1]) != [1, 1] or a.get("dilations", [1, 1]) != [1, 1] or a.get("group", 1) != 1 \
            or any(a.get("pads", [0, 0, 0, 0])) or a.get("auto_pad", b"NOTSET") not in (b"NOTSET", "NOTSET"):
        sys.exit(f"unsupported Conv attributes {a}")
    transpose = producer[n.input[0]]
    slice_in = producer[transpose.input[0]]
    unsq = producer[slice_in.input[0]]
    if (transpose.op_type, slice_in.op_type, unsq.op_type) != ("Transpose", "Slice", "Unsqueeze") \
            or attrs(transpose)["perm"] != [0, 2, 1, 3] or init_array(unsq.input[1]).tolist() != [3]:
        sys.exit(f"unexpected Conv input pattern at {n.name}")
    consumers = [c for c in g.node if n.output[0] in c.input]
    slice_out = consumers[0] if len(consumers) == 1 else None
    sq = [c for c in g.node if slice_out and slice_out.output[0] in c.input]
    if slice_out is None or slice_out.op_type != "Slice" or len(sq) != 1 or sq[0].op_type != "Squeeze" \
            or init_array(sq[0].input[1]).tolist() != [3]:
        sys.exit(f"unexpected Conv output pattern at {n.name}")
    sq = sq[0]
    x = unsq.input[0]
    _, length, chans = shapes[x]
    out_ch, in_ch, k, _ = w.shape
    if length is None or chans not in (None, in_ch):
        sys.exit(f"unexpected Conv input shape at {n.name}: {shapes[x]}")
    out_len = length - k + 1
    base = n.name + "/im2col"
    nodes = []
    cols = []
    for i in range(k):
        o = f"{base}/slice{i}"
        nodes.append(helper.make_node(
            "Slice",
            [x, add_init(f"{o}/starts", np.array([i], np.int64)), add_init(f"{o}/ends", np.array([i + out_len], np.int64)),
             add_init(f"{o}/axes", np.array([1], np.int64))],
            [o], name=o))
        cols.append(o)
    nodes.append(helper.make_node("Concat", cols, [base + "/cols"], name=base + "/concat", axis=2))
    # cols[n, t, i*C + c] = x[n, t+i, c]; weight row i*C + c holds W[o, c, i].
    wmat = np.ascontiguousarray(w[:, :, :, 0].transpose(2, 1, 0).reshape(k * in_ch, out_ch))
    nodes.append(helper.make_node("MatMul", [base + "/cols", add_init(base + "/weight", wmat)], [base + "/matmul"], name=base + "/matmul"))
    # Back to the [N, O, L'] layout the rest of the graph expects.
    nodes.append(helper.make_node("Transpose", [base + "/matmul"], list(sq.output), name=base + "/transpose", perm=[0, 2, 1]))
    replace[n.name] = nodes
    replace[sq.name] = []
    print(f"Conv {n.name}: im2col {k}x{in_ch} -> MatMul [{k * in_ch}, {out_ch}], output length {out_len}")

if not replace:
    sys.exit("nothing to patch")

new_nodes = []
for n in g.node:
    new_nodes.extend(replace.get(n.name, [n]))

# Drop nodes whose outputs are no longer used, then unused initializers.
needed = {o.name for o in g.output}
kept = []
for n in reversed(new_nodes):
    if any(o in needed for o in n.output):
        kept.append(n)
        needed.update(i for i in n.input if i)
kept.reverse()
print(f"nodes: {len(g.node)} -> {len(kept)}")
del g.node[:]
g.node.extend(kept)
used = {i for n in g.node for i in n.input}
keep_inits = [t for t in g.initializer if t.name in used]
print(f"initializers: {len(g.initializer)} -> {len(keep_inits)}")
del g.initializer[:]
g.initializer.extend(keep_inits)
del g.value_info[:]

onnx.checker.check_model(m)
onnx.save(m, args.dst)

if args.check:
    import onnxruntime as ort

    rng = np.random.default_rng(0)
    batch = rng.integers(0, 257, size=(64, 2048), dtype=np.int32)
    batch[:8, 1000:] = 256  # some padded rows, as for short files
    ref = ort.InferenceSession(args.src, providers=["CPUExecutionProvider"]).run(None, {"bytes": batch})[0]
    got = ort.InferenceSession(args.dst, providers=["CPUExecutionProvider"]).run(None, {"bytes": batch})[0]
    print(f"check: max |diff| {np.abs(ref - got).max():.3e}, argmax equal {bool((ref.argmax(1) == got.argmax(1)).all())}")
