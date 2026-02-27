import * as React from "react";

type TagProps = {
  name: string;
  count?: number;
  color?: string;
  onRemove?: () => void;
};

export function Tag({ name, count, color = "blue", onRemove }: TagProps) {
  return (
    <span style={{ color }}>
      {name}
      {count !== undefined && <span>({count})</span>}
      {onRemove && <button onClick={onRemove}>x</button>}
    </span>
  );
}
