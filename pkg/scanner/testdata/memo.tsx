import * as React from "react";

function Badge({ label }: { label: string }) {
  return <span className="badge">{label}</span>;
}

export const MemoBadge = React.memo(Badge);
