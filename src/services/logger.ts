import * as winston from "winston";
import { ui } from "../ui/console";
import { LOG_DIR } from "../config";
import * as fs from "fs";
import * as path from "path";

// Ensure the log directory exists
if (!fs.existsSync(LOG_DIR)) {
	fs.mkdirSync(LOG_DIR, { recursive: true });
}

// Custom transport for winston that uses our UI
class UiTransport extends winston.Transport {
	constructor(options?: winston.LoggerOptions) {
		super(options);
	}

	log(info: any, callback: () => void) {
		const { level, message } = info;
		ui.logEvent(level.toUpperCase(), message);

		if (callback) {
			callback();
		}
	}
}

const logger = winston.createLogger({
	level: "info",
	format: winston.format.combine(
		winston.format.timestamp({ format: "YYYY-MM-DD HH:mm:ss" }),
		winston.format.printf((info) => `${info.timestamp} [${info.level.toUpperCase()}]: ${info.message}`)
	),
	transports: [
		new winston.transports.File({ filename: path.join(LOG_DIR, "sync.log"), level: "info" }),
		new winston.transports.File({ filename: path.join(LOG_DIR, "sync-error.log"), level: "error" }),
		new winston.transports.File({ filename: path.join(LOG_DIR, "sync-warn.log"), level: "warn" }),
		new winston.transports.File({ filename: path.join(LOG_DIR, "sync-debug.log"), level: "debug" }),
	],
	exceptionHandlers: [new winston.transports.File({ filename: path.join(LOG_DIR, "exceptions.log") })],
	rejectionHandlers: [new winston.transports.File({ filename: path.join(LOG_DIR, "rejections.log") })],
});

// In development, use our custom UI transport instead of the default console
if (process.env.NODE_ENV !== "production") {
	logger.add(new UiTransport());
}

export default logger;
