# Concept 01 — Chunking in RAG, and what it means for video

## The one-sentence version

Retrieval returns **units**. Chunking is deciding what a unit *is* — and that decision
affects search quality more than the embedding model, the vector database, or the ranking
function combined.

Most people tune the model. The model is rarely the problem. The unit is.

---

## Why it dominates everything else

An embedding is a **single point** representing a whole chunk. That is the entire source
of the tension:

**Chunk too small** → the point is precise but starved of context. A sentence reading
"He refused." embeds into nothing useful. Retrieved, it answers no question. The
information needed to interpret it lived in the neighbouring sentences you cut away.

**Chunk too big** → the point becomes an *average of everything inside it*. A chunk
covering three topics embeds to a centroid that sits near none of them. This is the
failure people underestimate: it does not merely get noisier, the signal actively
cancels. A 10,000-word document embedded as one vector is nearly useless for retrieval —
it is equidistant from every query.

So there is a sweet spot, and it is not a number you can look up. It is **whatever unit
corresponds to one coherent idea in your domain**. In prose that is roughly a paragraph.
In code it is a function. In video, as we will see, it is a scene — not a frame, and not
a shot.

The practical consequence: **chunk on meaning, not on size.** Size is a symptom of
meaning, not a substitute for it.

---

## The strategies, worst to best

**1. Fixed-size (every N tokens, with overlap)**

Split every 512 tokens, overlap by 50. Trivial to implement, and it is what every tutorial
shows. It slices mid-sentence and mid-argument, so chunk boundaries fall in meaningless
places. The overlap exists purely to soften the damage — it is a patch over the fact that
the split ignored structure. Acceptable as a baseline. Never the right final answer.

**2. Structural / recursive**

Split on boundaries the content already has: sections, then paragraphs, then sentences,
recursing only when a piece is still too large. Now boundaries land where the author
intended a break. This is a large, cheap win over fixed-size and should be the floor.

**3. Semantic chunking**

Embed each sentence. Walk the document comparing consecutive embeddings. Where similarity
**drops sharply**, the topic changed — cut there. Boundaries are discovered from meaning
rather than assumed from structure. Costs an extra embedding pass. Worth it when the
content lacks reliable structural markers, which is exactly the situation in video: a film
has no paragraphs.

**4. Contextual retrieval**

The insight: a chunk pulled out of its document loses the context needed to interpret it.
So before embedding, **prepend a short description of where the chunk sits**:

```
"From the Q3 earnings report, in the section on European revenue:"
+ "Revenue declined 3% due to currency effects."
```

The embedding now carries what the raw text left implicit. This measurably reduces
retrieval failures and is cheap. It matters enormously for us, because a caption like
*"a man ties another man's ankles"* is far more findable when the embedded text is
*"In a rooftop confrontation between the hero and the villain: a man ties another man's
ankles."*

**5. Parent–child (small-to-big)** ← the pattern we use

Decouple the two jobs that people wrongly force one chunk to do:

- **Search over small units.** Small = precise = a clean embedding with one idea in it.
- **Return the large unit that contains the hit.** Large = enough context to be useful.

You index children, you match a child, you serve the parent. Precision and context stop
competing, because different units handle each.

This is the single most useful idea in the whole topic, and it maps onto our problem
almost too neatly.

---

## Mapping it to video

Text RAG chunks a document. We chunk **a film along time**. Same problem, different axis —
and the strategies transfer directly:

| Text RAG | Video equivalent | Verdict |
|---|---|---|
| Fixed-size split | Sample a frame every N seconds | Bad. Lands mid-cut, mid-motion. A frame at t=30 may be a transition blur meaning nothing. |
| Structural split | **Shot detection** — cut at camera cuts | Good. The cut is video's paragraph break, and it is *authored*, not inferred. |
| Semantic chunking | Merge adjacent shots whose embeddings stay similar | Good. This is how shots become scenes. |
| Contextual retrieval | Prepend film/scene context to each caption before embedding | Cheap, meaningful gain. |
| Parent–child | **child = shot, parent = scene** | This is our design. |

### Shot versus scene — the distinction that decides quality

A **shot** is one continuous camera take, typically 2–5 seconds.

A **scene** is a dramatic unit — one continuous piece of action — and a scene is usually
**many shots**, because the editor cuts back and forth between faces, angles and reactions
while a single event unfolds.

"Batman ties joker upside down" is not a shot. It is perhaps fifteen shots over ninety
seconds: a wide of the rooftop, a close-up of the rope, a reaction, a cut back to the
wide, and so on.

This is exactly why parent–child is the right structure:

- **Embed shots (children).** One shot is one visual idea. Its CLIP embedding is clean,
  and its caption describes one thing. Embedding a whole 90-second scene as a single
  vector would produce the blurred-centroid failure described above.
- **Return scenes (parents), and seek to `scene.start_ms`.** If we returned the matching
  *shot*, we would drop the viewer into the middle of the action — the rope close-up,
  eleven seconds in, with no idea how they got there. Users asking for a scene expect it
  to start at the beginning.

Getting this wrong is not a subtle degradation. It is the difference between a demo that
feels magic and one that feels broken, even with retrieval scoring identically well.

### How shots become scenes

Semantic chunking, applied to shots instead of sentences. Merge shot *i* with shot *i+1*
when they are:

- **temporally adjacent** (no long gap), and
- **visually similar** (embedding similarity above a threshold — same location, same
  lighting, same faces), or
- **linked by continuous dialogue** (a sentence spanning the cut means the action did too).

Stop merging at a hard visual discontinuity, a long silence, or a cap on scene length.

That similarity threshold is a real tuning knob and one of the few numbers worth tuning by
measurement rather than intuition — which is precisely what the labelled query set is for.

### One shot, three vectors

Each child shot carries more than one embedding, because it has several independent
signals:

1. **CLIP visual** — the keyframe, in the shared image/text space.
2. **Caption text** — the VLM's description, in a sentence-embedding space.
3. **Dialogue text** — the Whisper transcript overlapping the shot.

A query hits all three; results fuse by rank (RRF). Dialogue is useless during an action
sequence and captions are useless during a talking-heads exchange — which is the point.
The signals fail in different places, so fusing them covers more than any one alone.

---

## Naming collision — read this once, avoid weeks of confusion

The word **chunk** means two unrelated things in this project:

| Term | World | Meaning |
|---|---|---|
| **chunk** | transcode pipeline | ~30 s slice of video, a unit of *parallel ffmpeg work* |
| **chunk** | RAG | a unit of *retrieval* — which for us is a shot or a scene |

They share nothing: not boundaries, not sizes, not purposes. To keep them apart, this
project reserves "chunk" for the transcode meaning and always says **shot** or **scene**
for the retrieval meaning. Never write "chunk" when you mean a retrieval unit.

Alongside these sits **segment** — the 6-second HLS delivery unit. Three vocabularies,
three granularities, one shared timeline. See MASTER.md §7.

---

## What this buys us in the code

```
detect shots                    → structural chunking
embed each shot (3 vectors)     → children, indexed
merge shots into scenes         → semantic chunking
prepend context to captions     → contextual retrieval
query → match shot → return scene, seek to scene.start_ms   → parent-child
```

Five well-understood RAG techniques, each doing one job, all falling out of a single
question: *what is the unit?*

## The knobs worth measuring

Everything below is a number someone will be tempted to guess. Measure them against the
labelled query set instead:

- shot-detection sensitivity — too high shatters scenes, too low merges unrelated action
- shot→scene merge threshold — the main quality knob
- maximum scene length — the cap that stops runaway merging
- top-K per vector space before fusion
