# E2E Test Suite Status & Execution Guide (TEST_READY.md)

This E2E test suite validates the rewritten Audiobookshelf frontend/gateway port in Go, confirming parity with the original server design across all critical features.

---

## 1. Test Coverage Summary

The E2E test suite covers **N = 8 core features** and executes **94 distinct test cases** organized across four progressive validation tiers.

### Feature Coverage Matrix

| Ref | Core Feature | Tier 1 (Coverage) | Tier 2 (Edge/Error) | Tier 3 (Cross-Interaction) | Tier 4 (Workloads) |
| :--- | :--- | :---: | :---: | :---: | :---: |
| **F1** | Local Auth & Session Management | ✅ (5 cases) | ✅ (5 cases) | ✅ (3 cases) | ✅ (2 cases) |
| **F2** | OIDC Federated Auth | ✅ (5 cases) | ✅ (5 cases) | ✅ (1 case) | ✅ (1 case) |
| **F3** | Library & Folder Administration | ✅ (5 cases) | ✅ (5 cases) | ✅ (2 cases) | ✅ (2 cases) |
| **F4** | Library Scanning & Watching Tasks | ✅ (5 cases) | ✅ (5 cases) | ✅ (2 cases) | ✅ (1 case) |
| **F5** | Catalog Retrieval, Searching & Filtering| ✅ (5 cases) | ✅ (5 cases) | ✅ (2 cases) | ✅ (2 cases) |
| **F6** | Content Access & Downloading | ✅ (5 cases) | ✅ (5 cases) | ✅ (1 case) | ✅ (1 case) |
| **F7** | Audio Playback & HLS Transcoding | ✅ (5 cases) | ✅ (5 cases) | ✅ (2 cases) | ✅ (1 case) |
| **F8** | WebSocket Progress Sync & Bookmarks | ✅ (5 cases) | ✅ (5 cases) | ✅ (3 cases) | ✅ (2 cases) |

### Test Tiers
*   **Tier 1: Core Feature Verification (Happy Path)** — Verifies basic routes, normal user operations, valid configurations, and successful connection handling.
*   **Tier 2: Boundary & Corner Cases (Error Handling)** — Evaluates invalid credentials, token reuse/signature errors, restricted permission boundaries, traversal detection, and dirty disconnections.
*   **Tier 3: Pairwise Cross-Feature Interactions** — Assesses scenarios where actions in one feature trigger changes in another (e.g., database restores invalidating active websockets, settings updates revoking download permissions, scans hiding explicit media).
*   **Tier 4: Real-World Integrated Workloads** — Tests multi-step user scenarios (multi-device handover, offline playback synchronization, parental controls, backup restores, and concurrent transcode stresses).

---

## 2. Environment Prerequisites

Ensure the following tools are installed on your path:
1. **Go Compiler (1.20+)**
2. **FFmpeg & FFprobe** (required for scanning media tracks and HLS segment generation)
3. **SQLite3 library** (or pure Go modernc.org/sqlite as utilized in the test suite)

---

## 3. Running the Test Suite

All tests are fully automated and self-contained, starting the server binary in a separate process with isolated temporary config/metadata directories (`/tmp/abs-config-e2e` and `/tmp/abs-metadata-e2e`).

### Run All Test Tiers
To execute the complete 94-test catalog, run:
```bash
go test -v ./e2e/...
```

### Run Specific Features or Tiers
To isolate execution, filter by subtest names:
*   **Run only Tier 1 Core Feature tests:**
    ```bash
    go test -v ./e2e/... -run TestE2E_Tier1_CoreFeatures
    ```
*   **Run only HLS Transcoding tests:**
    ```bash
    go test -v ./e2e/... -run /F7_AudioPlayback
    ```
*   **Run only Parental Controls tests:**
    ```bash
    go test -v ./e2e/... -run TestE2E_Tier4_RealWorldWorkloads/5_ParentalRestrictions
    ```

---

## 4. Verification & Attestation

The suite uses live loopback networking and a mocked in-process OIDC provider to prevent dependency on external APIs.

*   **Host URL:** `http://localhost:3333`
*   **Mock OIDC URL:** Dynamically assigned via test server listener.
*   **Test DB:** SQLite database situated at `/tmp/abs-config-e2e/absdatabase.sqlite`.
