import { ChangeEvent, useState } from "react";
import "./App.css";
// import { Export } from "@backend/backend/Quotation"

function App() {
	const [formData, setFormData] = useState({
		price: "",
		quantity: ""
	});

	const rawTotal = Number(formData.price || 0) * Number(formData.quantity || 0);

	const formattedTotal = rawTotal.toLocaleString('de-DE');

	const handleChange = (e: ChangeEvent<HTMLInputElement>) => {
		const { name, value } = e.target;

		const cleanValue = value.replace(/\D/g, "");

		setFormData((prev) => ({
			...prev,
			[name]: cleanValue,
		}));
	};

	const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
		// Allow: Backspace, Tab, Enter, Escape, and Arrows
		const allowedKeys = ['Backspace', 'Tab', 'Enter', 'Escape', 'ArrowLeft', 'ArrowRight'];

		if (allowedKeys.includes(e.key)) return;

		// If it's not a number, prevent the input
		if (/\D/.test(e.key)) {
			e.preventDefault();
		}
	};

	return (
		<div>
			<nav></nav>
			<main>
				<div id="product-listing" className="p-[1.5rem] flex flex-col">
					<h3 className="text-[1.2rem] font-[600] mb-[1.5rem]">Product & Service Listing</h3>

					<div className="grid grid-cols-[3fr_1fr_0.5fr_1.2fr_1.2fr] gap-[1rem] align-middle font-[600] text-[0.85rem] text-[#505A67] p-[0_0.5rem_0.5rem_0.5rem] border-b-[1px] border-solid border-[#E0E5EC]">
						<p>Description</p>
						<p>UoM</p>
						<p>Qty</p>
						<p>Unit Price</p>
						<p>Total</p>
					</div>
					<div className="grid grid-cols-[3fr_1fr_0.5fr_1.2fr_1.2fr] gap-[1rem] align-middle font-[600] text-[0.85rem] text-[#505A67] p-[0.5rem_0.5rem_0.5rem_0.5rem]">
						<input type="text" placeholder="Services or Product Description" className="w-full p-[0.6rem_0.5rem] border-[1px] border-solid border-[#E0E5EC] rounded-md focus:border-[#010101] focus:outline-none focus:ring-0" />
						<input type="text" placeholder="e.g., Pcs" className="w-full p-[0.6rem_0.5rem] border-[1px] border-solid border-[#E0E5EC] rounded-md focus:border-[#010101] focus:outline-none focus:ring-0" />
						<input type="text" name="quantity" placeholder="1" value={formData.quantity} onChange={handleChange} onKeyDown={handleKeyDown} className="w-full p-[0.6rem_0.5rem] border-[1px] border-solid border-[#E0E5EC] rounded-md focus:border-[#010101] focus:outline-none focus:ring-0" />
						<input type="text" name="price" placeholder="e.g., 1500000" value={formData.price} onChange={handleChange} onKeyDown={handleKeyDown} className="w-full p-[0.6rem_0.5rem] border-[1px] border-solid border-[#E0E5EC] rounded-md focus:border-[#010101] focus:outline-none focus:ring-0" />
						<div className="w-full p-[0.6rem_0.5rem] border-[1px] border-solid border-[#E0E5EC] rounded-md focus:border-[#010101] focus:outline-none focus:ring-0">{formattedTotal}</div>
					</div>
					<button>Add Item</button>
				</div>
			</main>
		</div>
	)
}

export default App;
