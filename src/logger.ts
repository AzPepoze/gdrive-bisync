import * as winston from "winston";

const logger = winston.createLogger({
	level: "debug", // Set to debug to capture all levels
	format: winston.format.combine(
		winston.format.timestamp({ format: "YYYY-MM-DD HH:mm:ss" }),
		winston.format.printf((info) => `${info.timestamp} [${info.level.toUpperCase()}]: ${info.message}`)
	),
	transports: [
		new winston.transports.File({ filename: "sync.log", level: "info" }),
		new winston.transports.File({ filename: "sync-error.log", level: "error" }),
		new winston.transports.File({ filename: "sync-warn.log", level: "warn" }), // New file for warnings
	],
	exceptionHandlers: [
		new winston.transports.File({ filename: "exceptions.log" })
	],
	rejectionHandlers: [
		new winston.transports.File({ filename: "rejections.log" })
	]
});

if (process.env.NODE_ENV !== "production") {
	logger.add(
		new winston.transports.Console({
			format: winston.format.combine(
				winston.format.colorize(), // Add colors to console output
				winston.format.timestamp({ format: "YYYY-MM-DD HH:mm:ss" }),
				winston.format.printf((info) => `${info.timestamp} [${info.level.toUpperCase()}]: ${info.message}`)
			),
			level: "debug", // Console should show all levels in development
		})
	);
}

export default logger;