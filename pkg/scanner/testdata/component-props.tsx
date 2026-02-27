import * as React from "react";

// Pattern 1: ComponentProps<"input"> — HTML element props
function Input({ className, type, ...props }: React.ComponentProps<"input">) {
  return <input type={type} className={className} {...props} />;
}

// Pattern 2: ComponentProps<"button"> & custom intersection type
function SelectTrigger({
  className,
  size = "default",
  children,
  ...props
}: React.ComponentProps<"button"> & { size?: "sm" | "default" }) {
  return <button data-size={size} className={className} {...props}>{children}</button>;
}

// Pattern 3: ComponentPropsWithoutRef
function Checkbox({ className, ...props }: React.ComponentPropsWithoutRef<"input">) {
  return <input type="checkbox" className={className} {...props} />;
}

export { Input, SelectTrigger, Checkbox };
