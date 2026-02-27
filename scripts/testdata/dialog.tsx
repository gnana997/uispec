import * as React from "react";

interface DialogProps {
  open?: boolean;
  children: React.ReactNode;
}

interface DialogTriggerProps {
  children: React.ReactNode;
}

interface DialogContentProps {
  children: React.ReactNode;
}

export function Dialog({ open, children }: DialogProps) {
  return <div data-open={open}>{children}</div>;
}

export function DialogTrigger({ children }: DialogTriggerProps) {
  return <button>{children}</button>;
}

export function DialogContent({ children }: DialogContentProps) {
  return <div className="dialog-content">{children}</div>;
}
