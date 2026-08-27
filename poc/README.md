# poc/

Throwaway-by-design experiments. Each one answers a single question cheaply, before that
question gets expensive.

Code in here does **not** graduate into the pipeline. What graduates is the *answer* —
the numbers, the model choices, the tuned thresholds. Copying POC code into a service is
how prototypes become production by accident.

| POC | Question | Status |
|---|---|---|
| `video-search/` | Does semantic scene search work at all? | ready to run |

---

## video-search

`scene_search_poc.ipynb` — runs on **Kaggle**, free T4.

1. Settings → Accelerator → **GPU T4 x2**
2. Settings → Internet → **On**
3. Run all cells.

The GPU is deliberate. This tests whether the idea works, not whether a laptop can run it.
What fits on CPU is a later problem with a known fix (the `Embedder` seam in MASTER.md §2).

**Section 9 is manual and cannot be skipped.** You watch the clip and write ~15 queries
with their true timestamps. Without labels the notebook proves nothing — results always
look plausible when nothing is measuring them.

**The output that matters** is the ablation table (dialogue vs visual vs caption vs fused)
and the per-stage timings. Those two tables size the real pipeline: which passes are worth
their cost, and whether borrowed GPU is needed from day one.

Test content must be **legally clean** — Blender open movies, or public domain. Nothing
here can end up in a README screenshot or a demo otherwise.

## Test content

The retrieval spike currently runs against a 5-minute **Iron Man jet-chase clip** (720p30) sitting
in the project root. It was chosen because it is action-dense and dialogue-sparse — if retrieval
works on it, the result is not quietly coming from the transcript, which is the failure mode that
makes a demo look smart until someone queries a silent scene.

Two different bars apply here, and conflating them is how people get this wrong:

- **Private evaluation** — measuring recall against a file on your own disk. Fine.
- **Distribution** — repo, README, screenshots, a demo shown to an interviewer. Not fine.

So all media is gitignored (`*.mp4`, `*.mkv`, `*.mov`, ...), and the shipped demo runs on CC-BY
Blender open movies. The notebook keeps `SOURCE_URLS` wired to those so it still works with no
local file present.
