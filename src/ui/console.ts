import ora, { Ora } from "ora";
import logger from "../services/logger";

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
		logger.debug("UI: Starting spinner.");
		this.spinner.start();
	}

	stop() {
		logger.debug("UI: Stopping spinner.");
		if (this.countdownInterval) {
			clearInterval(this.countdownInterval);
			this.countdownInterval = null;
			logger.debug("UI: Cleared countdown interval.");
		}
		if (this.spinner.isSpinning) {
			this.spinner.stop();
		}
	}

	logEvent(level: string, message: string) {
		logger.debug(`UI: Processing logEvent - Level: ${level}, Message: ${message}`);
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
		logger.debug(`UI: Updating status to: ${status}`);
		if (this.countdownInterval) {
			clearInterval(this.countdownInterval);
			this.countdownInterval = null;
			logger.debug("UI: Cleared countdown interval due to status update.");
		}
		this.spinner.text = status;
	}

	startIdleCountdown(durationInMs: number) {
		ui.start();
		logger.debug(`UI: Starting idle countdown for ${durationInMs}ms.`);
		if (this.countdownInterval) {
			clearInterval(this.countdownInterval);
			logger.debug("UI: Cleared existing countdown interval.");
		}

		let remaining = durationInMs;

		const updateCountdown = () => {
			if (remaining <= 0) {
				this.updateStatus("Idle. Preparing for next scan...");
				if (this.countdownInterval) clearInterval(this.countdownInterval);
				logger.debug("UI: Countdown finished.");
				return;
			}
			const minutes = Math.floor(remaining / 1000 / 60);
			const seconds = Math.floor((remaining / 1000) % 60);
			const paddedSeconds = seconds.toString().padStart(2, "0");
			this.spinner.text = `Idle. Next scan in ${minutes}:${paddedSeconds}...`;
			remaining -= 1000;
			logger.debug(`UI: Countdown update - ${minutes}:${paddedSeconds}`);
		};

		updateCountdown(); // Initial display
		this.countdownInterval = setInterval(updateCountdown, 1000);
	}

	stopIdleCountdown() {
		logger.debug("UI: Stopping idle countdown.");
		if (this.countdownInterval) {
			clearInterval(this.countdownInterval);
			this.countdownInterval = null;
			logger.debug("UI: Cleared countdown interval.");
		}
	}
}

export const ui = new InteractiveUI();
