# Codebase Change Statement: sing

**Current Codebase:** `zhfreal/sing` (`dev-possible-patched`, Tag: `v0.5.9+patch2`, Commit: `d6d53d30c9019e879dea30ab9903e0eac7d289ee`)  
**Original Upstream Baseline:** `MetaCubeX/sing` (Branch `dev-possible`, Tag: `v0.5.9`, Commit: `d9dc96d1ce3ac4f2c2fe0596bdec36137f9a18b9`)  
**Repository Location:** `./sing-mine`  
**Status:** Production-Ready, Race-Verified (`go test -race ./...` passing 100%)

---

## 1. Executive Summary

This document records the delta between `zhfreal/sing @ dev-possible-patched` and the upstream baseline `MetaCubeX/sing @ v0.5.9` (`d9dc96d1`).

The modifications focus on dependency alignment and concurrency/resource-cleanup hardening in the NTP service (`common/ntp/service.go`).

---

## 2. Bug Fixes & Concurrency Remediations

### 2.1 NTP Pause Callback Leak & `clockOffset` Data Race (`common/ntp/service.go`)
* **Pause Callback Unregistration on `Close()`**:
  - Updated `(*Service).Close()` to unregister `s.pauseCallback` via `s.pause.UnregisterCallback(s.pauseCallback)` and clear `s.pauseCallback = nil` when `s.pause != nil && s.pauseCallback != nil`, preventing stopped NTP services from leaking callbacks in `pause.Manager`.
* **Race-Free Atomic `clockOffset`**:
  - Converted `Service.clockOffset` from `time.Duration` to `atomic.Int64`, updating `TimeFunc()` (`s.clockOffset.Load()`) and `update()` (`s.clockOffset.Store(int64(response.ClockOffset))`) to eliminate data races between periodic NTP syncs and concurrent `TimeFunc()` callers.

---

## 3. Modified Files Matrix

| File Path | Status | Change Category | Summary of Delta Relative to Upstream `v0.5.9` (`d9dc96d1`) |
| :--- | :--- | :--- | :--- |
| `common/ntp/service.go` | **Modified** | **Concurrency & Leak Fix** | Unregisters `s.pauseCallback` in `Close()` and converts `clockOffset` to `atomic.Int64`. |
| `go.mod` / `go.sum` | **Modified** | **Dependencies** | Updated Go toolchain directive to `go 1.26.0` and bumped `golang.org/x/sys` to `v0.50.0` (from `v0.5.9+patch1`). |
