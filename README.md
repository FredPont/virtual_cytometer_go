# Single-Cell Virtual Cytometer (Go + Fyne Port)

[![License: GPL v3](https://img.shields.io/badge/License-GPL%20v3-blue.svg)](https://www.gnu.org/licenses/gpl-3.0)
[![Go Version](https://img.shields.io/badge/Go-1.22+-blue.svg)](https://go.dev/)
[![Platform](https://img.shields.io/badge/Platform-Desktop-lightgrey.svg)](https://fyne.io/)

A native desktop port of the **Single-Cell Virtual Cytometer** (originally written in JS/Plotly) to **Go** using the **Fyne** toolkit.

## 📖 Description

**Single-Cell Virtual Cytometer** is open-source software designed for the straightforward visualization and exploration of multimodal single-cell datasets, similar to flow cytometry analysis.

This specific repository is a **high-performance port** of the original JavaScript implementation. By moving from a web-based environment to a native Go application, this version offers:
* **Enhanced Memory Management:** Ability to handle millions of cells via direct rasterization.
* **Smoother Interaction:** Native desktop responsiveness for zooming and panning.
* **Advanced Gating:** Improved quadrant gating and sequential selection tools.

## ✨ Features

* **Multi-panel Synchronization:** Cross-plot selection (lasso or rectangular brush) in one panel automatically highlights the same cells in the other.
* **High-Performance Rendering:** Points are rasterized directly into an image buffer, preventing memory bloat even with massive datasets.
* **Advanced Gating Tools:**
    * Sequential (freeze) gating.
    * Cumulative "gate only" display.
    * Quadrant-gate mode with real-time live counts and percentages.
* **Robust Data Import:** Intelligent detection of delimiters (Tab, Semicolon, Comma) for CSV/TSV files.

## 🎥 Demo Videos

Demo videos of the software in action can be found in the supplemental data of the [reference article](https://doi.org/10.1093/nargab/lqaa025).

## 📝 How to Cite

If you use this software in your research, please cite the original work:

> [**DOI: 10.1093/nargab/lqaa025**](https://doi.org/10.1093/nargab/lqaa025)

## 🚀 Installation & Usage

### Prerequisites

* [Go](https://go.dev/) (version 1.22 or higher)
* A C compiler (e.g., `gcc` on Linux/macOS, or `MinGW` on Windows) for the Fyne graphics drivers.

### Quick Start

1. **Clone the repository:**
   ```bash
   git clone https://github.com/YOUR_USERNAME/scvc-go-port.git
   cd scvc-go-port

## 🛠 Compilation via Makefile

This project includes a `Makefile` to automate cross-platform builds using [fyne-cross](https://github.com/fyne-io/fyne-cross). This method uses Docker to ensure the correct C toolchains (like MinGW for Windows) are used for native graphics.

### Prerequisites

To use the automated build commands, you must have the following installed:
* **Go** (version 1.19 or higher)
* **Docker** (running and accessible by your user)
* **fyne-cross** installed via:
  ```bash
  go install github.com/fyne-io/fyne-cross@latest