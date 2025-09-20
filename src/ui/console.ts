import ora, { Ora } from "ora";

class InteractiveUI {
	private spinner: Ora;
	private countdownInterval: NodeJS.Timeout | null = null;

	constructor() {
		this.spinner = ora({
			text: "Initializing...",
			spinner: "dots",
			color: "cyan",
		});
	}

	start() {
		this.spinner.start();
	}

	stop() {
		if (this.countdownInterval) {
			clearInterval(this.countdownInterval);
			this.countdownInterval = null;
		}
		if (this.spinner.isSpinning) {
			this.spinner.stop();
		}
	}

	logEvent(level: string, message: string) {
		const colors: { [key: string]: string } = {
			INFO: "\x1b[32m",
			WARN: "\x1b[33m",
			ERROR: "\x1b[31m",
			SUCCESS: "\x1b[36m",
			DEBUG: "\x1b[90m",
		};
		const color = colors[level.toUpperCase()] || "\x1b[0m";
		const icon =
			{
				INFO: "ℹ",
				WARN: "⚠",
				ERROR: "✖",
				SUCCESS: "✔",
				DEBUG: "⚙",
			}[level.toUpperCase()] || " ";

		this.spinner.stopAndPersist({
			symbol: `${color}${icon}\x1b[0m`,
			text: message,
		});
		// Restart spinner only if it was running
		if (!this.spinner.isSpinning) {
			this.spinner.start();
		}
	}

	updateStatus(status: string) {
		if (this.countdownInterval) {
			clearInterval(this.countdownInterval);
			this.countdownInterval = null;
		}
		this.spinner.text = status;
	}

	startIdleCountdown(durationInMs: number) {
		if (this.countdownInterval) {
			clearInterval(this.countdownInterval);
		}

		let remaining = durationInMs;

		const updateCountdown = () => {
			if (remaining <= 0) {
				this.updateStatus("Idle. Preparing for next scan...");
				if (this.countdownInterval) clearInterval(this.countdownInterval);
				return;
			}
			const minutes = Math.floor(remaining / 1000 / 60);
			const seconds = Math.floor((remaining / 1000) % 60);
			const paddedSeconds = seconds.toString().padStart(2, "0");
			this.spinner.text = `Idle. Next scan in ${minutes}:${paddedSeconds}...`;
			remaining -= 1000;
		};

		updateCountdown(); // Initial display
		this.countdownInterval = setInterval(updateCountdown, 1000);
	}
}

export const ui = new InteractiveUI();
