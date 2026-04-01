# Future Improvements

Things deliberately out of scope for v1 but worth revisiting.

---

## Detection gaps

### Init container CrashLoopBackOff
Kubernetes never sets `State.Waiting.Reason = "CrashLoopBackOff"` on init containers — it only appears on regular containers. When an init container crashes repeatedly the pod shows `Init:CrashLoopBackOff` as a pod-level phase string, but there is no clean per-container signal to hook into. Detecting this would require a heuristic like `restartCount > threshold` on init containers, which is less reliable. Currently Klarity misses non-OOM init container crash loops entirely.
