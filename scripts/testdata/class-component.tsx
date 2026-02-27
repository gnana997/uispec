import React from "react";

interface CounterProps {
  initialCount?: number;
}

interface CounterState {
  count: number;
}

export class Counter extends React.Component<CounterProps, CounterState> {
  state = { count: this.props.initialCount || 0 };

  render() {
    return <div>{this.state.count}</div>;
  }
}
