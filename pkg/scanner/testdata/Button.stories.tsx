import type { Meta, StoryObj } from "@storybook/react";
import { Button } from "./button";

const meta = {
  title: "Components/Button",
  component: Button,
  tags: ["autodocs"],
} satisfies Meta<typeof Button>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Primary: Story = {
  args: {
    variant: "default",
    size: "lg",
    children: "Click me",
  },
};

export const Secondary: Story = {
  args: {
    variant: "destructive",
    size: "sm",
  },
};

export const WithRender: Story = {
  render: (args) => (
    <div style={{ display: "flex", gap: "8px" }}>
      <Button {...args} />
    </div>
  ),
  args: {
    variant: "outline",
  },
};
