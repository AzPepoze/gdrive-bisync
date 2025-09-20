import * as winston from "winston";
import { ui } from "../ui/console";

// Custom transport for winston that uses our UI
class UiTransport extends winston.Transport {
	log(info: any, callback: () => void) {
		setImmediate(() => {
			this.emit("logged", info);
		});

		const { level, message } = info;
		ui.logEvent(level.toUpperCase(), message);

		if (callback) {
			callback();
		}
	}
}

const logger = winston.createLogger({
	level: "info", // Set default level to info to avoid spamming the UI
	format: winston.format.combine(
		winston.format.timestamp({ format: "YYYY-MM-DD HH:mm:ss" }),
		winston.format.printf((info) => `${info.timestamp} [${info.level.toUpperCase()}]: ${info.message}`)
	),
	transports: [
		new winston.transports.File({ filename: "sync.log", level: "info" }),
		new winston.transports.File({ filename: "sync-error.log", level: "error" }),
		new winston.transports.File({ filename: "sync-warn.log", level: "warn" }),
		new winston.transports.File({ filename: "sync-debug.log", level: "debug" }),
	],
	exceptionHandlers: [new winston.transports.File({ filename: "exceptions.log" })],
	rejectionHandlers: [new winston.transports.File({ filename: "rejections.log" })],
});

// In development, use our custom UI transport instead of the default console
if (process.env.NODE_ENV !== "production") {
	logger.add(new UiTransport());
}

export default logger;