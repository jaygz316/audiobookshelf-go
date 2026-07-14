# Plan: Graceful Shutdown of HLS Transcoding Streams to Prevent Process Leaks

We will implement a graceful shutdown mechanism for the active HLS transcoding streams to terminate running FFmpeg subprocesses when the server exits.

## Modified Files
1. `internal/hls/hls.go`: Add `Close()` method to `StreamManager` to terminate all active streams.
2. `internal/handlers/managers.go`: Expose `ShutdownStreamManager()` to shut down the package-private `streamManager`.
3. `main.go`: Call `handlers.ShutdownStreamManager()` in the graceful shutdown signal handler.
4. `internal/hls/hls_test.go`: Add a unit test to verify that `StreamManager.Close()` correctly shuts down all active streams.
