// Sample TSX file for parser testing
import React from 'react';

interface Props {
  name: string;
  age: number;
}

const Greeting: React.FC<Props> = ({ name, age }) => {
  return (
    <div className="greeting">
      <h1>Hello, {name}!</h1>
      <p>You are {age} years old.</p>
    </div>
  );
};

export default Greeting;
