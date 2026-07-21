# Implementation Plan - Series Stack Cascading Cards Refinement

## Target Goal
Refine the Series Stack Cascading Cards across `/series` grid, Author Series lists, and Series Details views to ensure 3D reflections, count badge symmetry, dynamic progress bar rendering from backend payload, and smooth mobile viewport fanning.

## Proposed Changes
1. **`frontend/css/components.css`**:
   - Add `.library-grid .series-cover-front` to the `-webkit-box-reflect` rule so that series cards in the main `/series` grid show book cover reflections.
   - Adjust `.series-cover-stack` and `.series-detail-cover-stack` styles for high-contrast count badge and responsive hover fanning.

2. **`frontend/js/authors.js`**:
   - Update `createSeriesCard`:
     - Synchronously check `series.progress` (returned by `/api/libraries/:id/series`) to set the series progress bar width immediately without waiting for async `progressCache` resolution.
     - Enforce exact badge positioning (`top-1 right-1`) and styling (`bg-accent text-primary text-[10px] font-bold px-2 py-0.5 rounded-full z-30 shadow-md`).
   - Update `loadSeriesDetails`:
     - Add the count badge (`top-1 right-1`) to the `.series-detail-cover-stack` header element.
