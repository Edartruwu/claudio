# OpenAI Speech-to-Text (Whisper) API — April 2026

## Models Available

### Transcription Models
- **`gpt-4o-transcribe`** — Latest high-quality model; supports `json` / `text` response formats; supports prompts & logprobs
- **`gpt-4o-mini-transcribe`** — Lighter variant of gpt-4o-transcribe; same parameter support as gpt-4o-transcribe
- **`gpt-4o-transcribe-diarize`** — Speaker diarization model; supports `json`, `text`, and `diarized_json` formats; requires `chunking_strategy` for audio > 30s; does not support prompts, logprobs, or `timestamp_granularities[]`
- **`whisper-1`** — Legacy model; supports `json`, `text`, `srt`, `verbose_json`, `vtt` formats; supports `timestamp_granularities[]` (word/segment level)

### Translation Model
- **`whisper-1`** — Only model for `/v1/audio/translations` endpoint (translates to English only)

---

## REST API Endpoints

### Transcriptions
```
POST /v1/audio/transcriptions
```

### Translations
```
POST /v1/audio/translations
```

---

## Parameters (Transcriptions)

### Required
- **`file`** — Audio file (multipart/form-data)
- **`model`** — Model name (string: `whisper-1`, `gpt-4o-transcribe`, `gpt-4o-mini-transcribe`, `gpt-4o-transcribe-diarize`)

### Optional
- **`language`** — ISO 639-1 or 639-3 language code (e.g., `en`, `de`, `fr`)
- **`prompt`** — Text to improve transcription accuracy; max ~224 tokens (whisper-1) or more (gpt-4o models); useful for acronyms, domain-specific terms
- **`response_format`**
  - `json` (default) — Returns `{"text": "..."}`
  - `text` — Plain text only
  - `srt` / `vtt` — Subtitle formats (whisper-1 only)
  - `verbose_json` — Detailed format with timing (whisper-1 only)
  - `diarized_json` — Speaker segments with speaker labels (gpt-4o-transcribe-diarize only)
- **`timestamp_granularities[]`** — Array of `"word"` or `"segment"` (whisper-1 only; requires `response_format=verbose_json`)
- **`stream`** — Boolean; streams transcript events (true/false; not supported in whisper-1)
- **`chunking_strategy`** — `"auto"` or VAD config (required for gpt-4o-transcribe-diarize when audio > 30 seconds)
- **`known_speaker_names[]`** — Array of speaker names (gpt-4o-transcribe-diarize only; up to 4 speakers)
- **`known_speaker_references[]`** — Data URLs of 2–10 second reference audio clips (gpt-4o-transcribe-diarize only)
- **`logprobs`** — Boolean; include token log probabilities (gpt-4o models)

### Authentication
- **Header**: `Authorization: Bearer $OPENAI_API_KEY`

---

## Supported File Formats & Limits

### Audio Formats
- `mp3`, `mp4`, `mpeg`, `mpga`, `m4a`, `wav`, `webm`

### File Size
- **Max 25 MB** per upload

### Longer Files
- Split files into ≤25 MB chunks (use PyDub or similar)
- Avoid splitting mid-sentence to preserve context
- For diarization, use `chunking_strategy: "auto"` for automatic segmentation

---

## New Features & Highlights (2026)

### 1. Speaker Diarization
- **Model**: `gpt-4o-transcribe-diarize`
- **Response format**: `diarized_json` (returns `segments` array with `speaker`, `text`, `start`, `end`)
- **Known speaker matching**: Pass reference clips + speaker names to map speaker IDs
- **Example output**:
  ```json
  {
    "segments": [
      {"speaker": "agent", "text": "Hello", "start": 0.5, "end": 1.2},
      {"speaker": "Unknown_1", "text": "Hi there", "start": 1.3, "end": 2.1}
    ]
  }
  ```

### 2. Word-Level Timestamps (whisper-1 only)
- **Parameter**: `timestamp_granularities: ["word"]`
- **Format**: `response_format: "verbose_json"`
- **Output**: Includes `words` array with word-level start/end times
- **Use case**: Video editing, word-level precision

### 3. GPT-4o Models with Prompting
- **gpt-4o-transcribe** & **gpt-4o-mini-transcribe** support `prompt` parameter for context injection
- Improved accuracy for proper nouns, domain-specific terms, acronyms
- More capable than whisper-1 prompting (no strict token limit on context)

### 4. Streaming Transcriptions
- **Parameter**: `stream: true`
- **Supported models**: gpt-4o-transcribe, gpt-4o-mini-transcribe (NOT whisper-1)
- **Events**: `transcript.text.delta` (incremental) → `transcript.text.done` (final)
- **Logprobs**: Include confidence scores via `include: ["logprobs"]`

### 5. Realtime Transcription API
- **WebSocket**: `wss://api.openai.com/v1/realtime?intent=transcription`
- **Models**: whisper-1, gpt-4o-transcribe, gpt-4o-mini-transcribe
- **Features**: Server-side VAD (voice activity detection), low-latency streaming

---

## Response Formats by Model

| Model | Formats | Timestamp Support | Diarization |
|-------|---------|-------------------|-------------|
| `whisper-1` | json, text, srt, vtt, verbose_json | word/segment (verbose_json) | ❌ |
| `gpt-4o-transcribe` | json, text | ❌ | ❌ |
| `gpt-4o-mini-transcribe` | json, text | ❌ | ❌ |
| `gpt-4o-transcribe-diarize` | json, text, diarized_json | ❌ | ✓ |

---

## Supported Languages

99+ languages including: English, Mandarin, Spanish, French, German, Japanese, Korean, Arabic, Portuguese, Russian, Hindi, and more. Minimum quality threshold: < 50% word error rate (WER).

---

## Authentication

All requests require the `Authorization` header:
```
Authorization: Bearer $OPENAI_API_KEY
```

Obtain API key from [OpenAI API Dashboard](https://platform.openai.com/account/api-keys).

---

## Separate Endpoints

| Endpoint | Purpose | Model(s) |
|----------|---------|----------|
| `POST /v1/audio/transcriptions` | Transcribe audio in original language | whisper-1, gpt-4o-transcribe*, gpt-4o-mini-transcribe*, gpt-4o-transcribe-diarize* |
| `POST /v1/audio/translations` | Translate audio to English | whisper-1 only |

*new in 2026

---

## Summary

As of April 2026, OpenAI offers **four transcription models**: the legacy `whisper-1` (with timestamp granularities), and three new GPT-4o variants providing higher quality, prompting support, streaming, and speaker diarization. File size is capped at 25 MB; the `/v1/audio/transcriptions` endpoint accepts audio in 7 formats. The new `gpt-4o-transcribe-diarize` model with diarized_json output is the headliner feature for multi-speaker scenarios.
