<div align="center">

# 🚀 EVE Flipper

### *The Ultimate Market Tool for EVE Online Traders*

[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://go.dev/)
[![React](https://img.shields.io/badge/React-18-61DAFB?style=for-the-badge&logo=react&logoColor=black)](https://react.dev/)
[![TypeScript](https://img.shields.io/badge/TypeScript-5-3178C6?style=for-the-badge&logo=typescript&logoColor=white)](https://www.typescriptlang.org/)
[![License](https://img.shields.io/badge/License-MIT-green?style=for-the-badge)](LICENSE)

<br/>

[![GitHub stars](https://img.shields.io/github/stars/ilyaux/Eve-flipper?style=for-the-badge&logo=github&color=yellow)](https://github.com/ilyaux/Eve-flipper/stargazers)
[![GitHub release](https://img.shields.io/github/v/release/ilyaux/Eve-flipper?style=for-the-badge&logo=github&color=blue)](https://github.com/ilyaux/Eve-flipper/releases/latest)
[![Downloads](https://img.shields.io/github/downloads/ilyaux/Eve-flipper/total?style=for-the-badge&logo=github&color=brightgreen)](https://github.com/ilyaux/Eve-flipper/releases)
[![Last Commit](https://img.shields.io/github/last-commit/ilyaux/Eve-flipper?style=for-the-badge&logo=github)](https://github.com/ilyaux/Eve-flipper/commits/master)

<br/>

**Station Trading** · **Hauling Routes** · **Contract Flipping** · **Industry Analysis**

[📥 Download](https://github.com/ilyaux/Eve-flipper/releases) · 
[📚 Documentation](https://github.com/ilyaux/Eve-flipper/wiki) · 
[🐛 Report Bug](https://github.com/ilyaux/Eve-flipper/issues)

<br/>

<img src="assets/screenshot-station.png" alt="EVE Flipper Screenshot" width="800"/>

</div>

---

## ✨ Features

<table>
<tr>
<td width="50%">

### 📊 Trading Tools
- **Station Trading Pro** — Same-station flipping with EVE Guru metrics
- **Radius Scan** — Find deals within jump range
- **Region Arbitrage** — Cross-region price differences
- **Contract Scanner** — Evaluate contracts vs market
- **Route Builder** — Optimal multi-hop trade routes

</td>
<td width="50%">

### 🏭 Industry Analysis
- **Production Chain Optimizer** — Buy vs Build calculator
- **Material Tree** — Visualize complete production chains
- **Shopping Lists** — Auto-generated buy lists
- **Cost Calculations** — ME/TE, system index, facility bonuses

</td>
</tr>
<tr>
<td width="50%">

### 🛡️ Risk Management
- **Scam Detection** — VWAP-based risk scoring
- **Bait Order Detection** — Identifies manipulation
- **Volume Analysis** — Liquidity warnings
- **Historical Data** — Price trend analysis

</td>
<td width="50%">

### 🎮 Quality of Life
- **EVE SSO Login** — View your orders & wallet
- **Scan History** — Save & restore results
- **Keyboard Shortcuts** — Power user workflow
- **Dark Theme** — Easy on the eyes

</td>
</tr>
</table>

---

## 🖼️ Screenshots

<div align="center">
<table>
<tr>
<td align="center"><b>Station Trading</b></td>
<td align="center"><b>Route Builder</b></td>
<td align="center"><b>Industry Optimizer</b></td>
</tr>
<tr>
<td><a href="assets/screenshot-station.png"><img src="assets/screenshot-station.png" width="280"/></a></td>
<td><a href="assets/screenshot-routes.png"><img src="assets/screenshot-routes.png" width="280"/></a></td>
<td><a href="assets/screenshot-radius.png"><img src="assets/screenshot-radius.png" width="280"/></a></td>
</tr>
</table>
<sub>Click to enlarge</sub>
</div>

---

## 🚀 Quick Start

### Download

Grab the latest release for your platform:

| Platform | Download |
|:--------:|:--------:|
| <img src="https://img.icons8.com/color/48/windows-10.png" width="24"/> **Windows** | [`eve-flipper-windows-amd64.exe`](https://github.com/ilyaux/Eve-flipper/releases/latest) |
| <img src="https://img.icons8.com/color/48/linux.png" width="24"/> **Linux** | [`eve-flipper-linux-amd64`](https://github.com/ilyaux/Eve-flipper/releases/latest) |
| <img src="https://img.icons8.com/color/48/mac-os.png" width="24"/> **macOS** | [`eve-flipper-darwin-arm64`](https://github.com/ilyaux/Eve-flipper/releases/latest) |

### Run

```bash
# Windows
eve-flipper-windows-amd64.exe

# Linux/macOS
chmod +x eve-flipper-*
./eve-flipper-linux-amd64
```

Open **http://127.0.0.1:13370** in your browser 🎉

> **Note:** First launch downloads SDE data (~1-2 minutes)

---

## 📖 Documentation

| Guide | Description |
|-------|-------------|
| 🏁 [Getting Started](https://github.com/ilyaux/Eve-flipper/wiki/Getting-Started) | Installation & first scan |
| 💹 [Station Trading](https://github.com/ilyaux/Eve-flipper/wiki/Station-Trading) | Master same-station trading |
| 🏭 [Industry Optimizer](https://github.com/ilyaux/Eve-flipper/wiki/Industry-Chain-Optimizer) | Buy vs Build analysis |
| 📊 [Metrics Reference](https://github.com/ilyaux/Eve-flipper/wiki/Metrics-Reference) | CTS, VWAP, SDS explained |
| 🔐 [EVE SSO Login](https://github.com/ilyaux/Eve-flipper/wiki/EVE-SSO-Login) | Connect your character |
| 🛠️ [Building from Source](https://github.com/ilyaux/Eve-flipper/wiki/Building-from-Source) | Compile yourself |

---

## ⌨️ Keyboard Shortcuts

| Shortcut | Action |
|:--------:|--------|
| `Ctrl + S` | Start/Stop scan |
| `Alt + 1-6` | Switch tabs |
| `Ctrl + W` | Open Watchlist |
| `Ctrl + H` | Open History |
| `Escape` | Close modals |

---

## 🛠️ Tech Stack

<div align="center">

| Backend | Frontend | Data |
|:-------:|:--------:|:----:|
| ![Go](https://img.shields.io/badge/Go-00ADD8?style=flat-square&logo=go&logoColor=white) | ![React](https://img.shields.io/badge/React-61DAFB?style=flat-square&logo=react&logoColor=black) | ![SQLite](https://img.shields.io/badge/SQLite-003B57?style=flat-square&logo=sqlite&logoColor=white) |
| ![Fiber](https://img.shields.io/badge/net/http-00ADD8?style=flat-square&logo=go&logoColor=white) | ![TypeScript](https://img.shields.io/badge/TypeScript-3178C6?style=flat-square&logo=typescript&logoColor=white) | ![ESI](https://img.shields.io/badge/EVE_ESI-1D1D1D?style=flat-square&logo=eve-online&logoColor=white) |
| Single binary | ![Tailwind](https://img.shields.io/badge/Tailwind-06B6D4?style=flat-square&logo=tailwindcss&logoColor=white) | Real-time data |

</div>

---

## 🤝 Contributing

Contributions are welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

```bash
git clone https://github.com/ilyaux/Eve-flipper.git
cd Eve-flipper
make build  # or .\make.ps1 build on Windows
```

---

## 👥 Contributors

<div align="center">

<a href="https://github.com/ilyaux/Eve-flipper/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=ilyaux/Eve-flipper&max=100" />
</a>

<br/><br/>

[![Contributors](https://img.shields.io/github/contributors/ilyaux/Eve-flipper?style=for-the-badge&color=blue)](https://github.com/ilyaux/Eve-flipper/graphs/contributors)
[![Forks](https://img.shields.io/github/forks/ilyaux/Eve-flipper?style=for-the-badge&color=lightgrey)](https://github.com/ilyaux/Eve-flipper/network/members)
[![Issues](https://img.shields.io/github/issues/ilyaux/Eve-flipper?style=for-the-badge&color=yellow)](https://github.com/ilyaux/Eve-flipper/issues)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen?style=for-the-badge)](https://github.com/ilyaux/Eve-flipper/pulls)

</div>

---

## 📄 License

This project is licensed under the **MIT License** — see [LICENSE](LICENSE) for details.

---

## ⚠️ Disclaimer

EVE Flipper is a third-party tool and is **not affiliated with or endorsed by CCP Games**. 
EVE Online and all related trademarks are property of CCP hf.

---

<div align="center">

**Made with ❤️ for New Eden traders**

⭐ Star this repo if you find it useful!

<br/>

<sub>
<a href="https://github.com/ilyaux/Eve-flipper/stargazers">Stars</a> · 
<a href="https://github.com/ilyaux/Eve-flipper/network/members">Forks</a> · 
<a href="https://github.com/ilyaux/Eve-flipper/issues">Issues</a> · 
<a href="https://github.com/ilyaux/Eve-flipper/releases">Releases</a>
</sub>

</div>

<!-- Keywords for search -->
<details>
<summary>Keywords</summary>

EVE Online market tool, EVE station trading, EVE hauling calculator, EVE arbitrage scanner, EVE market flipping, EVE ISK making tool, EVE trade route finder, EVE contract scanner, EVE profit calculator, EVE market analysis, Jita market scanner, EVE industry calculator, EVE production chain analyzer, EVE manufacturing cost calculator, EVE blueprint calculator, EVE buy vs build, New Eden trading, CCP ESI API tool

</details>
