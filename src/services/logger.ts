import * as winston from "winston";
import "winston-daily-rotate-file";
import DailyRotateFile from "winston-daily-rotate-file";
import TransportStream from "winston-transport";
import { ui } from "../ui/console";
import { LOG_DIR } from "../config";
import * as fs from "fs";
import * as path from "path";

// Ensure the log directory exists
if (fs.existsSync(LOG_DIR)) {
	fs.rmSync(LOG_DIR, { recursive: true, force: true });
}

fs.mkdirSync(LOG_DIR, { recursive: true });

// Define the transport options interface
interface UiTransportOptions extends TransportStream.TransportStreamOptions {
	// Add any specific options for your transport here
}

// Custom transport for winston that uses our UI
class UiTransport extends TransportStream {
	private debugLogBuffer: string[] = [];
	private readonly MAX_DEBUG_LOGS = 100;

	constructor(opts?: UiTransportOptions) {
		super(opts);
	}

	log(info: any, callback: () => void) {
		setImmediate(() => {
			this.emit("logged", info);
		});

		const level = info[Symbol.for("level")];
		const message = info[Symbol.for("message")];

		if (level === "debug") {
			this.debugLogBuffer.push(message);
			if (this.debugLogBuffer.length > this.MAX_DEBUG_LOGS) {
				this.debugLogBuffer.shift(); // Remove the oldest log
			}
			const formattedDebugLogs = this.debugLogBuffer.map((logMsg) => `[DEBUG]: ${logMsg}`).join("\n");
			ui.logEvent(level.toUpperCase(), formattedDebugLogs);
		} else {
			ui.logEvent(level.toUpperCase(), message);
		}

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
		new DailyRotateFile({
			filename: path.join(LOG_DIR, "sync-%DATE%.log"),
			datePattern: "YYYY-MM-DD",
			zippedArchive: true,
			maxSize: "100k", // Max size of 100KB
			maxFiles: 1, // Retain only 1 log file
			level: "info",
		}),
		new DailyRotateFile({
			filename: path.join(LOG_DIR, "sync-error-%DATE%.log"),
			datePattern: "YYYY-MM-DD",
			zippedArchive: true,
			maxSize: "100k", // Max size of 100KB
			maxFiles: 1, // Retain only 1 log file
			level: "error",
		}),
		new DailyRotateFile({
			filename: path.join(LOG_DIR, "sync-warn-%DATE%.log"),
			datePattern: "YYYY-MM-DD",
			zippedArchive: true,
			maxSize: "100k", // Max size of 100KB
			maxFiles: 1, // Retain only 1 log file
			level: "warn",
		}),
		new DailyRotateFile({
			filename: path.join(LOG_DIR, "sync-debug-%DATE%.log"),
			datePattern: "YYYY-MM-DD",
			zippedArchive: true,
			maxSize: "100k", // Max size of 100KB
			maxFiles: 1, // Retain only 1 log file
			level: "debug",
		}),
	],
	exceptionHandlers: [new winston.transports.File({ filename: path.join(LOG_DIR, "exceptions.log") })],
	rejectionHandlers: [new winston.transports.File({ filename: path.join(LOG_DIR, "rejections.log") })],
});

// In development, use our custom UI transport instead of the default console
// if (process.env.NODE_ENV !== "production") {
// 	logger.add(new UiTransport());
// }

export default logger;
