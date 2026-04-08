import "./App.css";
import Random from "./Random";
import { Generate } from "@backend/backend/Quotation"

function App() {
	const handleSubmit = async () => {
		const status = await Generate()
		console.log(status)
	}

	return (
		<div className="min-h-screen bg-white grid grid-cols-1 place-items-center justify-items-center mx-auto py-8">
			<div className="text-blue-900 text-2xl font-bold font-mono">
				<button onClick={() => handleSubmit()} className="content-center">generate</button>
			</div>
			<div className="w-fit max-w-md">
				<a href="https://wails.io" target="_blank"></a>
			</div>
			<Random />
		</div >
	);
}

export default App;
