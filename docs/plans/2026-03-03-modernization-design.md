# Design Doc: GemFactory 2026 Modernization (Deep System Optimization)

**Date:** 2026-03-03
**Status:** Approved
**Topic:** Performance & Efficiency via Go 1.25+ Features

## Overview
GemFactory requires a modernization effort to leverage Go 1.25 capabilities, specifically focusing on memory efficiency and processing speed for its scraper and background workers. This "Deep System Optimization" approach affects all layers from data acquisition to storage.

## Architecture & Data
- **String Interning (`unique` package)**: Implement `unique.Handle[string]` for high-cardinality data:
    - Artist Names
    - Source URLs
    - Spotify/YouTube links
- **Memory Efficiency**: Reduce heap allocations by interning strings at the source (Scraper) and maintaining handles through the Service and Storage layers.

## Components & Data Flow
- **Scraper Streaming (Iterators)**: 
    - Convert `ParseKProfilesMonthlyPage` to return `iter.Seq[Release]`.
    - Enable "as-soon-as-found" processing to reduce latency and memory spikes during massive paginated crawls.
- **Database Iterators**: Transition repository methods from returning large slices (e.g., `[]model.Release`) to returning iterators.
- **`sync.Pool` Integration**:
    - Manage `goquery.Document` instances to minimize GC pressure during heavy scraping cycles.
    - Reuse internal buffers for string cleaning and formatting.

## Error Handling & Concurrency
- **Multi-error Aggregation**: Use `errors.Join` in `errgroup` tasks to capture and report all failures in concurrent scraping batches, rather than just the first one.
- **Modern Lifecycle Management**: Use `context.AfterFunc` for deterministic resource cleanup (e.g., stopping chromedp instances).
- **Rate Limiting & Throttling**: Implement strict `errgroup.SetLimit` based on system resources to prevent OOM during high-concurrency jobs.

## Success Criteria
- [ ] No regression in scraper accuracy.
- [ ] 30%+ reduction in peak memory usage during full-month parses.
- [ ] Zero string allocations for repeated artist names/links across different releases.
