import * as React from "react";

interface InputProps {
  placeholder?: string;
  value?: string;
  onChange?: (e: React.ChangeEvent<HTMLInputElement>) => void;
}

const Input = React.forwardRef<HTMLInputElement, InputProps>(
  ({ placeholder, value, onChange }, ref) => {
    return <input ref={ref} placeholder={placeholder} value={value} onChange={onChange} />;
  }
);

Input.displayName = "Input";

export { Input };
