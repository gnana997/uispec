import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";

// CVA variants — only used by MenuButton.
const menuButtonVariants = cva("flex items-center gap-2", {
  variants: {
    variant: {
      default: "bg-background",
      outline: "border border-input",
    },
    size: {
      default: "h-10 px-4",
      sm: "h-8 px-3",
      lg: "h-12 px-6",
    },
  },
  defaultVariants: {
    variant: "default",
    size: "default",
  },
});

// --- Multiple components in the same file ---

interface MenuProps {
  children: React.ReactNode;
}

export function Menu({ children }: MenuProps) {
  return <nav>{children}</nav>;
}

interface MenuItemProps {
  label: string;
  active?: boolean;
}

export function MenuItem({ label, active = false }: MenuItemProps) {
  return <div data-active={active}>{label}</div>;
}

interface MenuButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof menuButtonVariants> {
  asChild?: boolean;
}

export function MenuButton({ variant, size, asChild, ...props }: MenuButtonProps) {
  return (
    <button className={menuButtonVariants({ variant, size })} {...props} />
  );
}

interface MenuSeparatorProps {
  className?: string;
}

export function MenuSeparator({ className }: MenuSeparatorProps) {
  return <hr className={className} />;
}
