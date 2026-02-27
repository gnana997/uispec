import * as React from "react";

interface AlertProps {
  /** The main message to display in the alert. */
  message: string;
  /** The severity level of the alert. */
  severity?: "info" | "warning" | "error";
  /** Whether the alert can be dismissed by the user. */
  dismissible?: boolean;
  /** Callback fired when the alert is dismissed. */
  onDismiss?: () => void;
  /** @deprecated Use `message` instead. */
  text?: string;
}

export function Alert({ message, severity = "info", dismissible = false, onDismiss, text }: AlertProps) {
  return (
    <div role="alert" data-severity={severity}>
      <span>{message || text}</span>
      {dismissible && <button onClick={onDismiss}>Dismiss</button>}
    </div>
  );
}
